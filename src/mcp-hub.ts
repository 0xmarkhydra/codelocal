import { promises as fs } from "node:fs";
import path from "node:path";
import os from "node:os";
import { randomUUID } from "node:crypto";
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

const REGISTRY_VERSION = 1 as const;
const CATALOG_VERSION = 1 as const;
const MAX_CATALOG_TOOLS = Number(process.env.CODELOCAL_MCP_MAX_TOOLS ?? 5000);
const MAX_STDERR_TAIL = 16 * 1024;
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
  if (!['http:', 'https:'].includes(url.protocol)) throw new Error("Remote MCP URL must use http or https.");
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

export class McpHub {
  private sessions = new Map<string, ConnectedSession>();

  constructor(private workspaceRoot = process.cwd()) {
    this.workspaceRoot = normalizeRoot(workspaceRoot);
  }

  private async registry(): Promise<RegistryFile> {
    const value = await readJson<RegistryFile>(mcpStatePaths().registry, { version: REGISTRY_VERSION, servers: [] });
    if (value.version !== REGISTRY_VERSION || !Array.isArray(value.servers)) throw new Error("Unsupported CodeLocal MCP registry format.");
    return value;
  }

  private async catalogFile(): Promise<CatalogFile> {
    const value = await readJson<CatalogFile>(mcpStatePaths().catalog, { version: CATALOG_VERSION, tools: [] });
    if (value.version !== CATALOG_VERSION || !Array.isArray(value.tools)) throw new Error("Unsupported CodeLocal MCP catalog format.");
    return value;
  }

  async addServer(input: Omit<McpServerConfig, "addedAt" | "updatedAt">) {
    const registry = await this.registry();
    const previous = registry.servers.find((server) => server.name === input.name && server.scope === input.scope && (server.scope === "global" || server.workspaceRoot === normalizeRoot(input.workspaceRoot ?? this.workspaceRoot)));
    const normalized = normalizeConfig({ ...input, workspaceRoot: input.scope === "workspace" ? (input.workspaceRoot ?? this.workspaceRoot) : undefined }, previous);
    registry.servers = registry.servers.filter((server) => !(server.name === normalized.name && server.scope === normalized.scope && (server.scope === "global" || server.workspaceRoot === normalized.workspaceRoot)));
    registry.servers.push(normalized);
    registry.servers.sort((a, b) => `${a.scope}:${a.name}`.localeCompare(`${b.scope}:${b.name}`));
    await writeJsonAtomic(mcpStatePaths().registry, registry);
    await this.disconnect(normalized.name);
    return this.publicServer(normalized);
  }

  async removeServer(name: string, scope?: Scope) {
    validateName(name);
    const registry = await this.registry();
    const before = registry.servers.length;
    registry.servers = registry.servers.filter((server) => {
      if (server.name !== name) return true;
      if (scope && server.scope !== scope) return true;
      if (server.scope === "workspace" && server.workspaceRoot !== this.workspaceRoot) return true;
      return false;
    });
    const removed = before - registry.servers.length;
    if (removed) await writeJsonAtomic(mcpStatePaths().registry, registry);
    const catalog = await this.catalogFile();
    const activeServerNames = new Set(registry.servers.map((server) => server.name));
    const filtered = catalog.tools.filter((tool) => tool.server !== name || activeServerNames.has(name));
    if (filtered.length !== catalog.tools.length) await writeJsonAtomic(mcpStatePaths().catalog, { version: CATALOG_VERSION, tools: filtered });
    await this.disconnect(name);
    return { removed };
  }

  async listServers() {
    const registry = await this.registry();
    const visible = registry.servers.filter((server) => server.scope === "global" || server.workspaceRoot === this.workspaceRoot);
    const catalog = await this.catalogFile();
    return visible.map((server) => ({
      ...this.publicServer(server),
      toolsCached: catalog.tools.filter((tool) => tool.server === server.name).length,
      connected: this.sessions.has(server.name),
    }));
  }

  async serverInfo(name: string) {
    const config = await this.resolveServer(name);
    const catalog = await this.catalogFile();
    return {
      ...this.publicServer(config),
      connected: this.sessions.has(name),
      tools: catalog.tools.filter((tool) => tool.server === name),
    };
  }

  async probe(name: string) {
    const config = await this.resolveServer(name);
    const client = await this.getOrConnect(config);
    const tools = await this.fetchAllTools(client);
    await this.replaceCatalogForServer(name, tools);
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
    let catalog = await this.catalogFile();
    const servers = await this.listServers();
    const visibleNames = new Set(servers.filter((server) => server.enabled).map((server) => server.name));
    if (options.server && !catalog.tools.some((tool) => tool.server === options.server)) {
      await this.probe(options.server);
      catalog = await this.catalogFile();
    }
    const ranked = catalog.tools
      .filter((tool) => visibleNames.has(tool.server) && (!options.server || tool.server === options.server))
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
      catalogToolCount: catalog.tools.filter((tool) => visibleNames.has(tool.server)).length,
      installedServerCount: visibleNames.size,
      recommendation: ranked.length ? "Call mcp_tool_info before mcp_call when you need the exact input schema." : "Run `codelocal mcp probe <name>` for newly installed servers, or narrow the search query.",
    };
  }

  async toolInfo(server: string, tool: string) {
    await this.resolveServer(server);
    let catalog = await this.catalogFile();
    let found = catalog.tools.find((item) => item.server === server && item.name === tool);
    if (!found) {
      await this.probe(server);
      catalog = await this.catalogFile();
      found = catalog.tools.find((item) => item.server === server && item.name === tool);
    }
    if (!found) throw new Error(`MCP tool not found: ${server}.${tool}`);
    return found;
  }

  async callTool(server: string, tool: string, args: Record<string, unknown> = {}) {
    const config = await this.resolveServer(server);
    const info = await this.toolInfo(server, tool);
    const client = await this.getOrConnect(config);
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

  private async getOrConnect(config: McpServerConfig) {
    const existing = this.sessions.get(config.name);
    if (existing) return existing.client;
    const client = new Client({ name: "codelocal-mcp-hub", version: "1.0.0" });
    let transport: StdioClientTransport | StreamableHTTPClientTransport;
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
      const session: ConnectedSession = { client, transport, connectedAt: Date.now(), lastUsedAt: Date.now(), stderrTail: "" };
      stdio.stderr?.on("data", (chunk) => {
        session.stderrTail = (session.stderrTail + String(chunk)).slice(-MAX_STDERR_TAIL);
      });
      this.sessions.set(config.name, session);
    } else {
      const headers = materializeHeaders(config.headers);
      transport = new StreamableHTTPClientTransport(new URL(config.url!), headers ? { requestInit: { headers } } : undefined);
      this.sessions.set(config.name, { client, transport, connectedAt: Date.now(), lastUsedAt: Date.now(), stderrTail: "" });
    }
    try {
      await client.connect(transport);
      return client;
    } catch (error) {
      const session = this.sessions.get(config.name);
      this.sessions.delete(config.name);
      await client.close().catch(() => undefined);
      const stderr = session?.stderrTail.trim();
      throw new Error(`Failed to connect MCP ${config.name}: ${error instanceof Error ? error.message : String(error)}${stderr ? `\nMCP stderr:\n${stderr}` : ""}`);
    }
  }

  private async fetchAllTools(client: Client) {
    const tools: McpCatalogTool[] = [];
    let cursor: string | undefined;
    do {
      const result = await client.listTools(cursor ? { cursor } : undefined);
      for (const tool of result.tools) {
        tools.push({
          server: "",
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

  private async replaceCatalogForServer(server: string, tools: McpCatalogTool[]) {
    const catalog = await this.catalogFile();
    const others = catalog.tools.filter((tool) => tool.server !== server);
    const next = [...others, ...tools.map((tool) => ({ ...tool, server }))];
    if (next.length > MAX_CATALOG_TOOLS) throw new Error(`MCP catalog exceeds CODELOCAL_MCP_MAX_TOOLS=${MAX_CATALOG_TOOLS}.`);
    next.sort((a, b) => `${a.server}.${a.name}`.localeCompare(`${b.server}.${b.name}`));
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
