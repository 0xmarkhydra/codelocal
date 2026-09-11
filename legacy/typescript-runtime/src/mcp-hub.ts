import { promises as fs } from "node:fs";
import path from "node:path";
import os from "node:os";
import { createHash, randomUUID } from "node:crypto";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

type Scope = "global" | "workspace";
type TransportKind = "stdio" | "http";

export type McpEnvReference = {
  source: string;
};

export type McpHeaderReference = {
  source: string;
  prefix?: string;
};

export type McpServerConfig = {
  name: string;
  enabled: boolean;
  scope: Scope;
  workspaceRoot?: string;
  transport: TransportKind;
  command?: string;
  args?: string[];
  cwd?: string;
  env?: Record<string, McpEnvReference>;
  url?: string;
  headers?: Record<string, McpHeaderReference>;
  addedAt: number;
  updatedAt: number;
};

export type McpCatalogTool = {
  serverKey: string;
  server: string;
  name: string;
  title?: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
  outputSchema?: Record<string, unknown>;
  annotations?: Record<string, unknown>;
  discoveredAt: number;
};

type RegistryFile = {
  version: 1;
  servers: McpServerConfig[];
};

type CatalogFile = {
  version: 1;
  tools: McpCatalogTool[];
};

type ConnectedSession = {
  client: Client;
  transport: StdioClientTransport | StreamableHTTPClientTransport;
  connectedAt: number;
  lastUsedAt: number;
  stderrTail: string;
};

export type McpConnectGuard = (config: Readonly<McpServerConfig>) => void | Promise<void>;

const REGISTRY_VERSION = 1 as const;
const CATALOG_VERSION = 1 as const;
const MAX_CATALOG_TOOLS = Number(process.env.CODELOCAL_MCP_MAX_TOOLS ?? 5000);
const MAX_STDERR_TAIL = 16 * 1024;
const MCP_SESSION_IDLE_MS = Math.max(60_000, Number(process.env.CODELOCAL_MCP_SESSION_IDLE_MS ?? 10 * 60_000) || 10 * 60_000);
const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$/;

function normalizeRoot(value: string) {
  return path.resolve(value);
}

function stateRoot() {
  return process.env.CODELOCAL_STATE_DIR ?? path.join(os.homedir(), ".codelocal");
}

export function mcpStatePaths() {
  const dir = path.join(stateRoot(), "mcp");
  return {
    dir,
    registry: path.join(dir, "registry.json"),
    catalog: path.join(dir, "catalog.json"),
  };
}

async function ensurePrivateDir(dir: string) {
  await fs.mkdir(dir, { recursive: true, mode: 0o700 });
  if (process.platform !== "win32") await fs.chmod(dir, 0o700).catch(() => undefined);
}

async function readJson<T>(file: string, fallback: T): Promise<T> {
  try {
    return JSON.parse(await fs.readFile(file, "utf8")) as T;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return fallback;
    throw new Error(`Invalid CodeLocal MCP state file ${file}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

async function writeJsonAtomic(file: string, value: unknown) {
  await ensurePrivateDir(path.dirname(file));
  const temp = `${file}.${process.pid}.${randomUUID()}.tmp`;
  const payload = JSON.stringify(value, null, 2) + "\n";
  await fs.writeFile(temp, payload, { encoding: "utf8", mode: 0o600, flag: "wx" });
  if (process.platform !== "win32") await fs.chmod(temp, 0o600).catch(() => undefined);
  await fs.rename(temp, file);
}

function validateName(name: string) {
  if (!NAME_RE.test(name)) throw new Error("MCP name must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}.");
  return name;
}

function validateEnvName(name: string) {
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) throw new Error(`Invalid environment variable name: ${name}`);
  return name;
}

function isLoopback(hostname: string) {
  const host = hostname.replace(/^\[/, "").replace(/\]$/, "").toLowerCase();
  return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "0.0.0.0";
}

function validateRemoteUrl(raw: string) {
  const url = new URL(raw);
  if (!["http:", "https:"].includes(url.protocol)) throw new Error("Remote MCP URL must use http or https.");
  if (url.protocol === "http:" && !isLoopback(url.hostname) && process.env.CODELOCAL_MCP_ALLOW_INSECURE_HTTP !== "1") {
    throw new Error("Remote MCP must use HTTPS unless it is localhost. Set CODELOCAL_MCP_ALLOW_INSECURE_HTTP=1 only for trusted development endpoints.");
  }
  return url.toString();
}

function materializeEnv(refs: Record<string, McpEnvReference> | undefined) {
  if (!refs) return undefined;
  const output: Record<string, string> = {};
  for (const [target, ref] of Object.entries(refs)) {
    validateEnvName(target);
    validateEnvName(ref.source);
    const value = process.env[ref.source];
    if (value == null) throw new Error(`MCP requires environment variable ${ref.source} for ${target}.`);
    output[target] = value;
  }
  return output;
}

function materializeHeaders(refs: Record<string, McpHeaderReference> | undefined) {
  if (!refs) return undefined;
  const headers: Record<string, string> = {};
  for (const [header, ref] of Object.entries(refs)) {
    validateEnvName(ref.source);
    const value = process.env[ref.source];
    if (value == null) throw new Error(`MCP requires environment variable ${ref.source} for HTTP header ${header}.`);
    headers[header] = `${ref.prefix ?? ""}${value}`;
  }
  return headers;
}

function normalizeConfig(input: Omit<McpServerConfig, "addedAt" | "updatedAt">, previous?: McpServerConfig): McpServerConfig {
  const name = validateName(input.name.trim());
  const now = Date.now();
  if (input.transport === "stdio") {
    if (!input.command?.trim()) throw new Error("stdio MCP requires a command.");
    if (input.url) throw new Error("stdio MCP cannot also define a URL.");
  } else {
    if (!input.url) throw new Error("remote MCP requires a URL.");
    validateRemoteUrl(input.url);
    if (input.command) throw new Error("remote MCP cannot also define a command.");
  }
  return {
    ...input,
    name,
    enabled: input.enabled !== false,
    workspaceRoot: input.scope === "workspace" ? normalizeRoot(input.workspaceRoot ?? process.cwd()) : undefined,
    command: input.command?.trim(),
    args: input.args?.map(String),
    url: input.url ? validateRemoteUrl(input.url) : undefined,
    addedAt: previous?.addedAt ?? now,
    updatedAt: now,
  };
}

function configKey(server: McpServerConfig) {
  if (server.scope === "global") return `global:${server.name}`;
  const rootHash = createHash("sha256").update(server.workspaceRoot ?? "").digest("hex").slice(0, 20);
  return `workspace:${server.name}:${rootHash}`;
}

function effectiveServers(registry: RegistryFile, workspaceRoot: string) {
  const byName = new Map<string, McpServerConfig>();
  for (const server of registry.servers) {
    if (server.scope === "global") byName.set(server.name, server);
  }
  for (const server of registry.servers) {
    if (server.scope === "workspace" && server.workspaceRoot === workspaceRoot) byName.set(server.name, server);
  }
  return [...byName.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function tokens(value: string) {
  return value.toLowerCase().split(/[^a-z0-9_./:-]+/).filter(Boolean);
}

function searchScore(tool: McpCatalogTool, query: string) {
  const q = query.trim().toLowerCase();
  if (!q) return 1;
  const hayName = `${tool.server}.${tool.name}`.toLowerCase();
  const title = (tool.title ?? "").toLowerCase();
  const description = (tool.description ?? "").toLowerCase();
  const schema = JSON.stringify(tool.inputSchema ?? {}).toLowerCase();
  let score = 0;
  if (hayName === q || tool.name.toLowerCase() === q) score += 100;
  if (hayName.includes(q)) score += 40;
  if (title.includes(q)) score += 24;
  if (description.includes(q)) score += 12;
  for (const term of tokens(q)) {
    if (tool.name.toLowerCase() === term) score += 25;
    else if (tool.name.toLowerCase().includes(term)) score += 14;
    if (tool.server.toLowerCase().includes(term)) score += 9;
    if (title.includes(term)) score += 7;
    if (description.includes(term)) score += 4;
    if (schema.includes(term)) score += 1;
  }
  return score;
}

function safeConnectDetail(config: McpServerConfig) {
  if (config.transport === "stdio") return `stdio ${config.command ?? config.name}`;
  try {
    const url = new URL(config.url ?? "");
    url.username = "";
    url.password = "";
    url.search = "";
    url.hash = "";
    return `http ${url.toString()}`;
  } catch {
    return `http ${config.name}`;
  }
}

export class McpHub {
  private sessions = new Map<string, ConnectedSession>();
  private connecting = new Map<string, Promise<Client>>();
  private idleTimer: NodeJS.Timeout;

  constructor(private workspaceRoot = process.cwd(), private beforeConnect?: McpConnectGuard) {
    this.workspaceRoot = normalizeRoot(workspaceRoot);
    const sweepMs = Math.min(60_000, Math.max(10_000, Math.floor(MCP_SESSION_IDLE_MS / 4)));
    this.idleTimer = setInterval(() => { void this.pruneIdleSessions(); }, sweepMs);
    this.idleTimer.unref?.();
  }

  private async pruneIdleSessions() {
    const cutoff = Date.now() - MCP_SESSION_IDLE_MS;
    const stale = [...this.sessions.entries()].filter(([, session]) => session.lastUsedAt < cutoff).map(([name]) => name);
    await Promise.allSettled(stale.map((name) => this.disconnect(name)));
  }

  private async registry(): Promise<RegistryFile> {
    const value = await readJson<RegistryFile>(mcpStatePaths().registry, { version: REGISTRY_VERSION, servers: [] });
    if (value.version !== REGISTRY_VERSION || !Array.isArray(value.servers)) throw new Error("Unsupported CodeLocal MCP registry format.");
    return value;
  }

  private async catalogFile(): Promise<CatalogFile> {
    const value = await readJson<CatalogFile>(mcpStatePaths().catalog, { version: CATALOG_VERSION, tools: [] });
    if (value.version !== CATALOG_VERSION || !Array.isArray(value.tools)) throw new Error("Unsupported CodeLocal MCP catalog format.");
    const tools = value.tools.filter((tool) => typeof tool?.serverKey === "string" && typeof tool?.server === "string" && typeof tool?.name === "string");
    return { version: CATALOG_VERSION, tools };
  }

  async addServer(input: Omit<McpServerConfig, "addedAt" | "updatedAt">) {
    const registry = await this.registry();
    const previous = registry.servers.find((server) => server.name === input.name && server.scope === input.scope && (server.scope === "global" || server.workspaceRoot === normalizeRoot(input.workspaceRoot ?? this.workspaceRoot)));
    const normalized = normalizeConfig({ ...input, workspaceRoot: input.scope === "workspace" ? (input.workspaceRoot ?? this.workspaceRoot) : undefined }, previous);
    registry.servers = registry.servers.filter((server) => !(server.name === normalized.name && server.scope === normalized.scope && (server.scope === "global" || server.workspaceRoot === normalized.workspaceRoot)));
    registry.servers.push(normalized);
    registry.servers.sort((a, b) => `${a.scope}:${a.name}:${a.workspaceRoot ?? ""}`.localeCompare(`${b.scope}:${b.name}:${b.workspaceRoot ?? ""}`));
    await writeJsonAtomic(mcpStatePaths().registry, registry);
    const catalog = await this.catalogFile();
    const key = configKey(normalized);
    const filtered = catalog.tools.filter((tool) => tool.serverKey !== key);
    if (filtered.length !== catalog.tools.length) await writeJsonAtomic(mcpStatePaths().catalog, { version: CATALOG_VERSION, tools: filtered });
    await this.disconnect(normalized.name);
    return this.publicServer(normalized);
  }

  async removeServer(name: string, scope?: Scope) {
    validateName(name);
    const registry = await this.registry();
    const removedConfigs: McpServerConfig[] = [];
    const kept: McpServerConfig[] = [];
    for (const server of registry.servers) {
      const matchesName = server.name === name;
      const matchesScope = !scope || server.scope === scope;
      const visibleWorkspaceConfig = server.scope !== "workspace" || server.workspaceRoot === this.workspaceRoot;
      if (matchesName && matchesScope && visibleWorkspaceConfig) removedConfigs.push(server);
      else kept.push(server);
    }
    registry.servers = kept;
    if (removedConfigs.length) await writeJsonAtomic(mcpStatePaths().registry, registry);
    if (removedConfigs.length) {
      const removedKeys = new Set(removedConfigs.map(configKey));
      const catalog = await this.catalogFile();
      const filtered = catalog.tools.filter((tool) => !removedKeys.has(tool.serverKey));
      if (filtered.length !== catalog.tools.length) await writeJsonAtomic(mcpStatePaths().catalog, { version: CATALOG_VERSION, tools: filtered });
    }
    await this.disconnect(name);
    return { removed: removedConfigs.length };
  }

  async listServers() {
    const registry = await this.registry();
    const visible = effectiveServers(registry, this.workspaceRoot);
    const catalog = await this.catalogFile();
    return visible.map((server) => {
      const key = configKey(server);
      return {
        ...this.publicServer(server),
        toolsCached: catalog.tools.filter((tool) => tool.serverKey === key).length,
        connected: this.sessions.has(server.name),
      };
    });
  }

  async serverInfo(name: string) {
    const config = await this.resolveServer(name);
    const key = configKey(config);
    const catalog = await this.catalogFile();
    return {
      ...this.publicServer(config),
      connected: this.sessions.has(name),
      tools: catalog.tools.filter((tool) => tool.serverKey === key),
    };
  }

  async probe(name: string, authorizeConnect = false) {
    const config = await this.resolveServer(name);
    const client = await this.getOrConnect(config, authorizeConnect);
    const tools = await this.fetchAllTools(client);
    await this.replaceCatalogForServer(config, tools);
    return {
      server: this.publicServer(config),
      connected: true,
      toolCount: tools.length,
      tools: tools.slice(0, 100).map((tool) => ({ name: tool.name, title: tool.title, description: tool.description })),
      truncated: tools.length > 100,
    };
  }

  async searchTools(query: string, options: { limit?: number; server?: string; refresh?: boolean } = {}) {
    const limit = Math.max(1, Math.min(options.limit ?? 8, 50));
    if (options.refresh && options.server) await this.probe(options.server);
    const registry = await this.registry();
    const effective = effectiveServers(registry, this.workspaceRoot).filter((server) => server.enabled);
    const effectiveByName = new Map(effective.map((server) => [server.name, server]));
    if (options.server) {
      const config = await this.resolveServer(options.server);
      const key = configKey(config);
      const existing = await this.catalogFile();
      if (!existing.tools.some((tool) => tool.serverKey === key)) await this.probe(options.server);
    }
    const catalog = await this.catalogFile();
    const allowedKeys = new Set(effective.map(configKey));
    const ranked = catalog.tools
      .filter((tool) => allowedKeys.has(tool.serverKey) && (!options.server || tool.server === options.server))
      .map((tool) => ({ ...tool, score: searchScore(tool, query) }))
      .filter((tool) => !query.trim() || tool.score > 0)
      .sort((a, b) => b.score - a.score || `${a.server}.${a.name}`.localeCompare(`${b.server}.${b.name}`))
      .slice(0, limit);
    return {
      query,
      results: ranked.map((tool) => ({
        server: tool.server,
        tool: tool.name,
        title: tool.title,
        description: tool.description,
        score: tool.score,
        readOnlyHint: tool.annotations?.readOnlyHint === true,
      })),
      catalogToolCount: catalog.tools.filter((tool) => allowedKeys.has(tool.serverKey)).length,
      installedServerCount: effectiveByName.size,
      recommendation: ranked.length ? "Call mcp_tool_info before mcp_call when you need the exact input schema." : "Probe a newly installed MCP with explicit local approval, then search again.",
    };
  }

  async cachedToolInfo(server: string, tool: string) {
    const config = await this.resolveServer(server);
    const key = configKey(config);
    const catalog = await this.catalogFile();
    return catalog.tools.find((item) => item.serverKey === key && item.name === tool) ?? null;
  }

  async toolInfo(server: string, tool: string, authorizeConnect = false) {
    let found = await this.cachedToolInfo(server, tool);
    if (!found) {
      await this.probe(server, authorizeConnect);
      found = await this.cachedToolInfo(server, tool);
    }
    if (!found) throw new Error(`MCP tool not found: ${server}.${tool}`);
    return found;
  }

  async callTool(server: string, tool: string, args: Record<string, unknown> = {}, options: { authorizeConnect?: boolean } = {}) {
    const config = await this.resolveServer(server);
    const info = await this.toolInfo(server, tool, options.authorizeConnect === true);
    const client = await this.getOrConnect(config, options.authorizeConnect === true);
    const session = this.sessions.get(server);
    if (session) session.lastUsedAt = Date.now();
    const result = await client.callTool({ name: info.name, arguments: args });
    return {
      server,
      tool,
      readOnlyHint: info.annotations?.readOnlyHint === true,
      result,
    };
  }

  async disconnect(name: string) {
    const session = this.sessions.get(name);
    if (!session) return;
    this.sessions.delete(name);
    await session.client.close().catch(() => undefined);
  }

  async shutdown() {
    clearInterval(this.idleTimer);
    await Promise.allSettled([...this.connecting.values()]);
    await Promise.all([...this.sessions.keys()].map((name) => this.disconnect(name)));
  }

  private publicServer(server: McpServerConfig) {
    return {
      name: server.name,
      enabled: server.enabled,
      scope: server.scope,
      workspaceRoot: server.scope === "workspace" ? server.workspaceRoot : undefined,
      transport: server.transport,
      command: server.command,
      args: server.args,
      cwd: server.cwd,
      env: server.env ? Object.fromEntries(Object.entries(server.env).map(([key, ref]) => [key, ref.source])) : undefined,
      url: server.url,
      headers: server.headers ? Object.fromEntries(Object.entries(server.headers).map(([key, ref]) => [key, { source: ref.source, prefix: ref.prefix ? "[configured]" : undefined }])) : undefined,
      addedAt: server.addedAt,
      updatedAt: server.updatedAt,
    };
  }

  private async resolveServer(name: string) {
    validateName(name);
    const registry = await this.registry();
    const workspace = registry.servers.find((server) => server.name === name && server.scope === "workspace" && server.workspaceRoot === this.workspaceRoot);
    const global = registry.servers.find((server) => server.name === name && server.scope === "global");
    const config = workspace ?? global;
    if (!config) throw new Error(`MCP server not installed for this workspace: ${name}`);
    if (!config.enabled) throw new Error(`MCP server is disabled: ${name}`);
    return config;
  }

  private async getOrConnect(config: McpServerConfig, authorizeConnect = false) {
    const existing = this.sessions.get(config.name);
    if (existing) {
      existing.lastUsedAt = Date.now();
      return existing.client;
    }
    if (!authorizeConnect) await this.beforeConnect?.(config);

    const inFlight = this.connecting.get(config.name);
    if (inFlight) return inFlight;

    const connecting = this.connectNew(config).finally(() => {
      if (this.connecting.get(config.name) === connecting) this.connecting.delete(config.name);
    });
    this.connecting.set(config.name, connecting);
    return connecting;
  }

  private async connectNew(config: McpServerConfig) {
    const client = new Client({ name: "codelocal-mcp-hub", version: "1.0.0" });
    let transport: StdioClientTransport | StreamableHTTPClientTransport;
    let session: ConnectedSession;
    if (config.transport === "stdio") {
      const cwd = config.cwd ? (path.isAbsolute(config.cwd) ? config.cwd : path.resolve(config.scope === "workspace" ? this.workspaceRoot : process.cwd(), config.cwd)) : (config.scope === "workspace" ? this.workspaceRoot : undefined);
      const stdio = new StdioClientTransport({
        command: config.command!,
        args: config.args ?? [],
        cwd,
        env: materializeEnv(config.env),
        stderr: "pipe",
      });
      transport = stdio;
      session = { client, transport, connectedAt: Date.now(), lastUsedAt: Date.now(), stderrTail: "" };
      stdio.stderr?.on("data", (chunk) => {
        session.stderrTail = (session.stderrTail + String(chunk)).slice(-MAX_STDERR_TAIL);
      });
    } else {
      const headers = materializeHeaders(config.headers);
      transport = new StreamableHTTPClientTransport(new URL(config.url!), headers ? { requestInit: { headers } } : undefined);
      session = { client, transport, connectedAt: Date.now(), lastUsedAt: Date.now(), stderrTail: "" };
    }
    try {
      await client.connect(transport);
      this.sessions.set(config.name, session);
      return client;
    } catch (error) {
      await client.close().catch(() => undefined);
      const stderr = session.stderrTail.trim();
      throw new Error(`Failed to connect MCP ${config.name}: ${error instanceof Error ? error.message : String(error)}${stderr ? `\nMCP stderr:\n${stderr}` : ""}`);
    }
  }

  private async fetchAllTools(client: Client) {
    const tools: Omit<McpCatalogTool, "serverKey" | "server">[] = [];
    let cursor: string | undefined;
    do {
      const result = await client.listTools(cursor ? { cursor } : undefined);
      for (const tool of result.tools) {
        tools.push({
          name: tool.name,
          title: tool.title,
          description: tool.description,
          inputSchema: tool.inputSchema as Record<string, unknown>,
          outputSchema: tool.outputSchema as Record<string, unknown> | undefined,
          annotations: tool.annotations as Record<string, unknown> | undefined,
          discoveredAt: Date.now(),
        });
        if (tools.length > MAX_CATALOG_TOOLS) throw new Error(`MCP catalog exceeds CODELOCAL_MCP_MAX_TOOLS=${MAX_CATALOG_TOOLS}.`);
      }
      cursor = result.nextCursor;
    } while (cursor);
    return tools;
  }

  private async replaceCatalogForServer(server: McpServerConfig, tools: Omit<McpCatalogTool, "serverKey" | "server">[]) {
    const catalog = await this.catalogFile();
    const key = configKey(server);
    const others = catalog.tools.filter((tool) => tool.serverKey !== key);
    const next: McpCatalogTool[] = [...others, ...tools.map((tool) => ({ ...tool, serverKey: key, server: server.name }))];
    if (next.length > MAX_CATALOG_TOOLS) throw new Error(`MCP catalog exceeds CODELOCAL_MCP_MAX_TOOLS=${MAX_CATALOG_TOOLS}.`);
    next.sort((a, b) => `${a.serverKey}.${a.name}`.localeCompare(`${b.serverKey}.${b.name}`));
    await writeJsonAtomic(mcpStatePaths().catalog, { version: CATALOG_VERSION, tools: next });
  }
}

export function parseEnvReference(value: string): [string, McpEnvReference] {
  const [targetRaw, sourceRaw] = value.includes("=") ? value.split("=", 2) : [value, value];
  const target = validateEnvName(targetRaw.trim());
  const source = validateEnvName(sourceRaw.trim());
  return [target, { source }];
}

export function parseHeaderEnvReference(value: string): [string, McpHeaderReference] {
  const index = value.indexOf("=");
  if (index <= 0) throw new Error("Header env format must be Header-Name=ENV_VAR.");
  const header = value.slice(0, index).trim();
  const source = validateEnvName(value.slice(index + 1).trim());
  if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(header)) throw new Error(`Invalid HTTP header name: ${header}`);
  return [header, { source }];
}
