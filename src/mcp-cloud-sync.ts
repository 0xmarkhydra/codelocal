import { promises as fs } from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { mcpStatePaths, type McpServerConfig } from "./mcp-hub.js";

export type CloudMcpSyncEntry = {
  id: string;
  name: string;
  enabled: boolean;
  scope: "global" | "workspace";
  workspaceId?: string;
  transport: "stdio" | "http";
  config: Record<string, unknown>;
  requiredSecrets: string[];
  updatedAt: number;
};

type ManagedConfig = McpServerConfig & {
  managedBy?: "cloud";
  cloudInstallationId?: string;
  cloudWorkspaceId?: string;
};

type Registry = { version: 1; servers: ManagedConfig[] };

async function readRegistry(file: string): Promise<Registry> {
  try {
    const parsed = JSON.parse(await fs.readFile(file, "utf8")) as Registry;
    if (parsed.version !== 1 || !Array.isArray(parsed.servers)) throw new Error("Unsupported MCP registry format.");
    return parsed;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return { version: 1, servers: [] };
    throw error;
  }
}

async function writePrivate(file: string, value: unknown) {
  await fs.mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temp = `${file}.${process.pid}.${randomUUID()}.tmp`;
  await fs.writeFile(temp, JSON.stringify(value, null, 2) + "\n", { encoding: "utf8", mode: 0o600, flag: "wx" });
  if (process.platform !== "win32") await fs.chmod(temp, 0o600).catch(() => undefined);
  await fs.rename(temp, file);
}

function secretRefs(names: string[]) {
  return Object.fromEntries(names.filter((name) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(name)).map((name) => [name, { source: name }]));
}

function mapEntry(entry: CloudMcpSyncEntry, workspaceRoot: string): ManagedConfig {
  const now = Date.now();
  const common = {
    name: entry.name,
    enabled: entry.enabled,
    scope: entry.scope,
    workspaceRoot: entry.scope === "workspace" ? workspaceRoot : undefined,
    transport: entry.transport,
    addedAt: entry.updatedAt || now,
    updatedAt: entry.updatedAt || now,
    managedBy: "cloud" as const,
    cloudInstallationId: entry.id,
    cloudWorkspaceId: entry.workspaceId,
  };
  if (entry.transport === "stdio") {
    const command = String(entry.config.command ?? "").trim();
    if (!command || /[\r\n\0]/.test(command)) throw new Error(`Cloud MCP ${entry.name} has an invalid stdio command.`);
    const args = Array.isArray(entry.config.args) ? entry.config.args.map(String) : [];
    const cwd = typeof entry.config.cwd === "string" ? entry.config.cwd : undefined;
    const env = secretRefs(entry.requiredSecrets ?? []);
    return { ...common, command, args, cwd, env: Object.keys(env).length ? env : undefined };
  }
  const rawUrl = String(entry.config.url ?? "");
  const url = new URL(rawUrl);
  const host = url.hostname.replace(/^\[/, "").replace(/\]$/, "");
  const local = ["localhost", "127.0.0.1", "::1"].includes(host);
  if (url.protocol !== "https:" && !(url.protocol === "http:" && local)) throw new Error(`Cloud MCP ${entry.name} must use HTTPS unless it is localhost.`);
  return { ...common, url: url.toString() };
}

export async function syncCloudMcpInstallations(workspaceRoot: string, entries: CloudMcpSyncEntry[]) {
  const paths = mcpStatePaths();
  const registry = await readRegistry(paths.registry);
  // Replace cloud-global config and this workspace's cloud config only. A different
  // `codelocal .` process may be using the same machine-level registry for another
  // workspace, so its workspace-scoped cloud entries must remain intact.
  const preserved = registry.servers.filter((server) => {
    if (server.managedBy !== "cloud") return true;
    if (server.scope === "global") return false;
    return server.workspaceRoot !== workspaceRoot;
  });
  const cloud = entries.map((entry) => mapEntry(entry, workspaceRoot));
  const merged = [...preserved, ...cloud].sort((a, b) => {
    const aManaged = a.managedBy === "cloud" ? 1 : 0;
    const bManaged = b.managedBy === "cloud" ? 1 : 0;
    return `${a.scope}:${a.name}:${a.workspaceRoot ?? ""}:${aManaged}`.localeCompare(`${b.scope}:${b.name}:${b.workspaceRoot ?? ""}:${bManaged}`);
  });
  await writePrivate(paths.registry, { version: 1, servers: merged });
  // Tool schemas are cheap to rediscover and may have changed with cloud config.
  // Clearing cached schemas is conservative: other running workspaces may need to
  // re-probe, but no stale schema can be routed to a changed MCP definition.
  await writePrivate(paths.catalog, { version: 1, tools: [] });
  return {
    cloudManaged: cloud.length,
    preservedOtherWorkspaceCloud: preserved.filter((server) => server.managedBy === "cloud").length,
    localManaged: preserved.filter((server) => server.managedBy !== "cloud").length,
    total: merged.length,
  };
}
