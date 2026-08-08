import express, { type Request, type Response } from "express";
import http from "node:http";
import { randomUUID, timingSafeEqual } from "node:crypto";
import { WebSocketServer, type WebSocket } from "ws";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { isInitializeRequest } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { oauthRouter, requireMcpAuth } from "./oauth.js";
import { log, summarizeToolArgs } from "./log.js";
import { PROTOCOL_VERSION, MIN_PROTOCOL_VERSION, isSideEffectingTool, protocolCompatible } from "./protocol.js";
import { CloudDeviceStore } from "./cloud-device-store.js";
import { audit } from "./audit.js";
import { cloudStore } from "./cloud-store.js";
import { webAuthRouter, getWebIdentity, requireWebUser, verifyCsrf } from "./saas-auth.js";
import { createDashboardRouter } from "./dashboard.js";
import { authPage, escapeHtml } from "./web-ui.js";
import { bridgeMcpToolResult } from "./mcp-bridge.js";

const VERSION = "1.4.0-dev.0";
const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN ?? "";
const TOOL_TIMEOUT_MS = Number(process.env.TOOL_TIMEOUT_MS ?? 180_000);
const HEARTBEAT_MS = Number(process.env.CODELOCAL_HEARTBEAT_MS ?? 20_000);
const STALE_MS = Number(process.env.CODELOCAL_STALE_MS ?? 70_000);
const ALLOW_LEGACY_DEVICE_TOKEN = process.env.ALLOW_LEGACY_DEVICE_TOKEN === "1" && !!DEVICE_TOKEN;
const deviceStore = new CloudDeviceStore();

await cloudStore.init();

type ClientCapabilities = {
  filesystem?: boolean; git?: boolean; shell?: boolean; pty?: boolean; sandbox?: string; semanticProviders?: string[]; idempotency?: boolean; cancellation?: boolean; approvals?: boolean; mcpHub?: boolean;
};

type ClientRecord = {
  key: string;
  userId: string;
  deviceId: string;
  deviceName: string;
  workspaceId: string;
  workspaceName: string;
  projectRoot?: string;
  credentialId?: string;
  protocolVersion: number;
  capabilities: ClientCapabilities;
  ws: WebSocket;
  connectedAt: number;
  lastSeenAt: number;
};

type Pending = {
  resolve: (value: unknown) => void;
  reject: (error?: unknown) => void;
  timer: NodeJS.Timeout;
  tool: string;
  startedAt: number;
  clientKey: string;
  userId: string;
  abortCleanup?: () => void;
};

type TransportRecord = { transport: StreamableHTTPServerTransport; userId: string };

const clients = new Map<string, ClientRecord>();
const socketKeys = new Map<WebSocket, string>();
const pending = new Map<string, Pending>();
const transports: Record<string, TransportRecord> = {};

function clientKey(userId: string, deviceId: string, workspaceId: string) { return `${userId}::${deviceId}::${workspaceId}`; }
function textResult(value: unknown) { return { content: [{ type: "text" as const, text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }] }; }
function availableClients(userId: string) { return [...clients.values()].filter((client) => client.userId === userId && client.ws.readyState === client.ws.OPEN); }
function equalSecret(a: string, b: string) { const aa = Buffer.from(a); const bb = Buffer.from(b); return aa.length === bb.length && timingSafeEqual(aa, bb); }

function resolveClient(userId: string, selectedKey?: string | null) {
  if (selectedKey) {
    const client = clients.get(selectedKey);
    if (!client || client.userId !== userId || client.ws.readyState !== client.ws.OPEN) throw new Error(`Selected workspace is offline or unavailable: ${selectedKey}`);
    return client;
  }
  const online = availableClients(userId);
  if (online.length === 1) return online[0];
  if (!online.length) throw new Error("No local CodeLocal client is online for this account.");
  throw new Error("Multiple workspaces are online. Call list_workspaces then select_workspace first.");
}

async function callClient(userId: string, tool: string, args: unknown, selectedKey: string | null, options: { signal?: AbortSignal; sessionId?: string } = {}) {
  const client = resolveClient(userId, selectedKey);
  const requestId = randomUUID();
  const startedAt = Date.now();
  const idempotencyKey = isSideEffectingTool(tool) ? requestId : undefined;
  log("info", "tool.dispatch", { requestId, tool, userId, clientKey: client.key, args: summarizeToolArgs(tool, args), pending: pending.size });
  await audit({ event: "gateway.tool.dispatch", requestId, workspaceKey: client.key, tool });
  await cloudStore.audit(userId, "gateway.tool.dispatch", { requestId, tool }, client.deviceId, client.workspaceId).catch(() => undefined);

  const result = new Promise<unknown>((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(requestId);
      if (client.capabilities.cancellation) client.ws.send(JSON.stringify({ type: "tool_cancel", protocolVersion: PROTOCOL_VERSION, requestId, reason: "gateway timeout" }));
      reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`));
    }, TOOL_TIMEOUT_MS);
    const entry: Pending = { resolve, reject, timer, tool, startedAt, clientKey: client.key, userId };
    if (options.signal) {
      const onAbort = () => {
        if (!pending.has(requestId)) return;
        client.ws.send(JSON.stringify({ type: "tool_cancel", protocolVersion: PROTOCOL_VERSION, requestId, reason: "MCP request cancelled" }));
        clearTimeout(timer); pending.delete(requestId); reject(new Error("Tool request cancelled."));
      };
      options.signal.addEventListener("abort", onAbort, { once: true });
      entry.abortCleanup = () => options.signal?.removeEventListener("abort", onAbort);
    }
    pending.set(requestId, entry);
  });

  client.ws.send(JSON.stringify({ type: "tool_call", protocolVersion: PROTOCOL_VERSION, requestId, id: requestId, sessionId: options.sessionId, workspaceKey: client.key, tool, args, idempotencyKey, deadline: Date.now() + TOOL_TIMEOUT_MS }));
  return result;
}

function createMcpServer(userId: string) {
  const server = new McpServer({ name: "codelocal", version: VERSION });
  let selectedKey: string | null = null;
  const localTool = (name: string, title: string, description: string, inputSchema: Record<string, any>, handler: (args: any) => unknown | Promise<unknown>) => {
    server.registerTool(name, { title, description, inputSchema }, async (args: any) => textResult(await handler(args)));
  };
  const remote = (name: string, title: string, description: string, schema: Record<string, any>) => {
    server.registerTool(name, { title, description, inputSchema: schema }, async (args: any, extra: any) => {
      const result = await callClient(userId, name, args, selectedKey, { signal: extra?.signal, sessionId: extra?.sessionId });
      if (name === "mcp_call") {
        const bridged = bridgeMcpToolResult(result);
        return (bridged ?? textResult(result)) as any;
      }
      return textResult(result);
    });
  };

  localTool("list_devices", "List online devices", "List online CodeLocal devices and workspaces belonging to this account.", {}, async () => {
    const grouped = new Map<string, any[]>();
    for (const client of availableClients(userId)) {
      const list = grouped.get(client.deviceId) ?? [];
      list.push({ workspaceId: client.workspaceId, workspaceName: client.workspaceName, key: client.key, projectRoot: client.projectRoot ?? null, protocolVersion: client.protocolVersion, capabilities: client.capabilities, lastSeenAt: client.lastSeenAt });
      grouped.set(client.deviceId, list);
    }
    return { devices: [...grouped.entries()].map(([deviceId, workspaces]) => ({ deviceId, workspaces })) };
  });
  localTool("list_device_identities", "List paired devices", "List this account's paired device identities without secrets.", {}, () => deviceStore.listDevices(userId));
  localTool("revoke_device", "Revoke device", "Revoke one of this account's paired device credentials.", { credentialId: z.string().min(1) }, async (args) => ({ revoked: await revokeAndDisconnect(userId, args.credentialId) }));
  localTool("rename_device", "Rename device", "Rename one of this account's durable paired device identities.", { credentialId: z.string().min(1), deviceName: z.string().min(1).max(120) }, async (args) => ({ renamed: await deviceStore.rename(userId, args.credentialId, args.deviceName) }));
  localTool("list_workspaces", "List workspaces", "List online workspaces for this account and current selection.", {}, async () => ({ selectedWorkspace: selectedKey, workspaces: availableClients(userId).map((client) => ({ key: client.key, deviceId: client.deviceId, deviceName: client.deviceName, workspaceId: client.workspaceId, workspaceName: client.workspaceName, projectRoot: client.projectRoot ?? null, capabilities: client.capabilities })) }));
  localTool("select_workspace", "Select workspace", "Select the workspace used by subsequent coding tools in this MCP session.", { key: z.string().min(1) }, async (args) => {
    const client = clients.get(args.key);
    if (!client || client.userId !== userId || client.ws.readyState !== client.ws.OPEN) throw new Error(`Workspace unavailable: ${args.key}`);
    selectedKey = client.key;
    return { selected: client.key, deviceId: client.deviceId, workspaceId: client.workspaceId, workspaceName: client.workspaceName };
  });
  localTool("workspace_info", "Workspace info", "Show the currently selected CodeLocal workspace.", {}, async () => {
    const client = resolveClient(userId, selectedKey);
    return { selected: client.key, deviceId: client.deviceId, deviceName: client.deviceName, workspaceId: client.workspaceId, workspaceName: client.workspaceName, projectRoot: client.projectRoot ?? null, protocolVersion: client.protocolVersion, capabilities: client.capabilities, lastSeenAt: client.lastSeenAt };
  });

  remote("project_info", "Project info", "Inspect workspace capabilities, project map, semantic providers, sandbox and instructions. Call first.", {});
  remote("project_map", "Project map", "Return cached compact project structure, languages, frameworks, commands and roots.", { force: z.boolean().default(false) });
  remote("context_for_task", "Context for task", "Select likely relevant symbols/files for a task hint before broad repository scans.", { taskHint: z.string().min(1), limit: z.number().int().min(1).max(100).default(30) });
  remote("read_instructions", "Read instructions", "Read scoped AGENTS.md and supported coding instructions.", { path: z.string().default(".") });
  remote("list_files", "List files", "Gitignore-aware project listing. Sensitive paths remain blocked.", { path: z.string().default("."), maxDepth: z.number().int().min(0).max(20).default(4), includeIgnored: z.boolean().default(false) });
  remote("file_info", "File metadata", "Read metadata/hash without source content.", { path: z.string().min(1) });
  remote("read_file", "Read file", "Read a targeted UTF-8 file.", { path: z.string().min(1) });
  remote("read_file_range", "Read file range", "Read a targeted line range.", { path: z.string().min(1), startLine: z.number().int().min(1), endLine: z.number().int().min(1) });
  remote("read_files", "Read files", "Batch read targeted files.", { paths: z.array(z.string().min(1)).min(1).max(50) });
  remote("search_code", "Search code", "Text search fallback for literal/unknown queries. Prefer semantic tools for definitions/references.", { query: z.string().min(1), path: z.string().default("."), maxResults: z.number().int().min(1).max(1000).default(200), fixedStrings: z.boolean().default(false), includeIgnored: z.boolean().default(false) });
  remote("inspect_dependency", "Inspect dependency", "Inspect dependency metadata for Node/Python/Rust/Go.", { name: z.string().min(1), ecosystem: z.enum(["auto", "node", "python", "rust", "go"]).default("auto") });
  remote("read_dependency", "Read dependency", "Read a targeted installed Node dependency file.", { name: z.string().min(1), path: z.string().default("package.json"), startLine: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional(), ecosystem: z.string().optional() });
  remote("search_dependency", "Search dependency", "Search inside one installed Node dependency.", { name: z.string().min(1), query: z.string().min(1), maxResults: z.number().int().min(1).max(500).default(100), fixedStrings: z.boolean().default(false) });

  remote("semantic_info", "Semantic providers", "Show installed semantic providers and fallback mode.", {});
  remote("workspace_symbols", "Workspace symbols", "Find symbols across a polyglot workspace.", { query: z.string().default(""), limit: z.number().int().min(1).max(1000).default(200) });
  remote("find_symbol", "Find symbol", "Compatibility alias for workspace symbol lookup.", { query: z.string().default(""), limit: z.number().int().min(1).max(1000).default(200) });
  remote("document_symbols", "Document symbols", "Find structural symbols in a file using LSP/TypeScript/fallback.", { path: z.string().min(1), limit: z.number().int().min(1).max(1000).default(500) });
  const semanticPositionSchema = { path: z.string().optional(), line: z.number().int().min(1).optional(), column: z.number().int().min(1).optional(), name: z.string().optional(), query: z.string().optional(), limit: z.number().int().min(1).max(2000).optional() };
  remote("find_definition", "Find definition", "Find definitions using exact LSP position when supplied, otherwise name/fallback search.", semanticPositionSchema);
  remote("find_references", "Find references", "Find references using exact LSP position when supplied, otherwise name/fallback search.", semanticPositionSchema);
  remote("find_implementations", "Find implementations", "Find implementations using the file position semantic provider.", { path: z.string().min(1), line: z.number().int().min(1), column: z.number().int().min(1), limit: z.number().int().min(1).max(1000).default(200) });
  remote("get_hover", "Get hover", "Get type/signature/hover information at a file position.", { path: z.string().min(1), line: z.number().int().min(1), column: z.number().int().min(1) });
  remote("get_diagnostics", "Get diagnostics", "Get diagnostics from TypeScript or the file's LSP provider.", { path: z.string().optional(), limit: z.number().int().min(1).max(2000).default(500) });
  remote("get_callers", "Get callers", "Get TypeScript/JavaScript callers where semantic call graph is available.", { name: z.string().min(1), limit: z.number().int().min(1).max(1000).default(300) });
  remote("get_callees", "Get callees", "Get TypeScript/JavaScript callees where semantic call graph is available.", { name: z.string().min(1), limit: z.number().int().min(1).max(1000).default(300) });
  remote("get_import_graph", "Get import graph", "Get current lightweight import graph.", { limit: z.number().int().min(1).max(10000).default(2000) });

  remote("write_file", "Write file", "Create/overwrite a file with optional stale-hash protection.", { path: z.string().min(1), content: z.string(), expectedHash: z.string().optional() });
  remote("edit_file", "Exact edit", "Compatibility exact-text edit with optional hash protection.", { path: z.string().min(1), oldText: z.string().min(1), newText: z.string(), replaceAll: z.boolean().default(false), expectedHash: z.string().optional() });
  remote("apply_patch", "Apply patch", "Validate and apply a unified Git patch.", { patch: z.string().min(1) });
  remote("apply_edits", "Apply structured edits", "Transactionally validate and apply multiple hash-safe file/range edits with rollback on failure.", { files: z.array(z.object({ path: z.string().min(1), expectedHash: z.string().optional(), edits: z.array(z.object({ startOffset: z.number().int().min(0).optional(), endOffset: z.number().int().min(0).optional(), startLine: z.number().int().min(1).optional(), startColumn: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional(), endColumn: z.number().int().min(1).optional(), replacement: z.string() })).min(1) })).min(1).max(100) });
  remote("format_changed_files", "Format changed files", "Use installed project/language formatters without downloading tools.", { paths: z.array(z.string().min(1)).min(1).max(200) });
  remote("snapshot_diagnostics", "Snapshot diagnostics", "Create a before-change diagnostic baseline.", { paths: z.array(z.string()).default([]) });
  remote("verify_changes", "Verify changes", "Return diagnostics regression, recommended checks and Git diff after edits.", { paths: z.array(z.string()).default([]), baselineId: z.string().optional() });

  remote("git_status", "Git status", "Read branch and working tree status.", {});
  remote("git_diff", "Git diff", "Read working/staged diff.", { cached: z.boolean().default(false), path: z.string().optional() });
  remote("git_log", "Git log", "Read recent Git history.", { limit: z.number().int().min(1).max(100).default(20), path: z.string().optional() });
  remote("git_show", "Git show", "Read a commit/ref summary.", { ref: z.string().default("HEAD") });
  remote("git_blame", "Git blame", "Read blame for a file/range.", { path: z.string().min(1), startLine: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional() });
  remote("git_file_history", "Git file history", "Read follow-renames file history.", { path: z.string().min(1), limit: z.number().int().min(1).max(100).default(30) });
  remote("git_stage", "Stage files", "Approval-gated Git stage operation.", { paths: z.array(z.string().min(1)).min(1).max(200) });
  remote("git_unstage", "Unstage files", "Approval-gated Git unstage operation.", { paths: z.array(z.string().min(1)).min(1).max(200) });
  remote("git_commit", "Commit staged changes", "Approval-gated commit. Optionally assert the exact staged path set.", { message: z.string().min(1).max(5000), expectedPaths: z.array(z.string()).optional() });
  remote("git_push", "Push commits", "Approval-gated non-force push.", { remote: z.string().optional(), branch: z.string().optional(), force: z.boolean().default(false) });

  remote("sandbox_info", "Sandbox info", "Show active native/best-effort/policy-only sandbox backend.", {});
  remote("sandbox_smoke_test", "Sandbox smoke test", "Run a local sandbox smoke test.", {});
  remote("run_command", "Run command", "Compatibility guarded command wrapper. Prefer exec_start/pty_start for long-lived processes.", { command: z.string().min(1), cwd: z.string().default("."), yieldMs: z.number().int().min(0).max(10000).default(1000), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remote("exec_start", "Start process", "Start guarded non-PTY process with independent stdout/stderr cursors.", { command: z.string().min(1), cwd: z.string().default("."), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remote("exec_poll", "Poll process", "Read incremental stdout/stderr.", { processId: z.string().min(1), stdoutCursor: z.number().int().min(0).optional(), stderrCursor: z.number().int().min(0).optional() });
  remote("exec_write", "Write process stdin", "Write to a running process stdin.", { processId: z.string().min(1), input: z.string() });
  remote("exec_signal", "Signal process", "Send a supported signal to a process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGINT", "SIGKILL"]).default("SIGTERM") });
  remote("exec_cancel", "Cancel process", "Cancel a running process.", { processId: z.string().min(1), reason: z.string().optional() });
  remote("exec_kill", "Kill process", "Terminate a process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });
  remote("pty_start", "Start PTY", "Start a guarded true PTY when node-pty is installed; otherwise safely falls back to a normal process.", { command: z.string().min(1), cwd: z.string().default("."), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remote("pty_poll", "Poll PTY", "Read incremental PTY output.", { processId: z.string().min(1), stdoutCursor: z.number().int().min(0).optional(), stderrCursor: z.number().int().min(0).optional() });
  remote("pty_write", "Write PTY", "Write input to PTY/process.", { processId: z.string().min(1), input: z.string() });
  remote("pty_resize", "Resize PTY", "Resize a true PTY.", { processId: z.string().min(1), cols: z.number().int().min(10).max(500), rows: z.number().int().min(5).max(300) });
  remote("pty_signal", "Signal PTY", "Signal PTY/process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGINT", "SIGKILL"]).default("SIGTERM") });
  remote("pty_kill", "Kill PTY", "Terminate PTY/process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });
  remote("process_list", "List processes", "List CodeLocal-started processes.", {});
  remote("process_poll", "Poll process compatibility", "Compatibility poll tool.", { processId: z.string().min(1), cursor: z.number().int().min(0).optional() });
  remote("process_write", "Write process compatibility", "Compatibility stdin tool.", { processId: z.string().min(1), input: z.string() });
  remote("process_kill", "Kill process compatibility", "Compatibility termination tool.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });

  remote("mcp_list", "List installed MCPs", "List MCP extensions installed in the selected local CodeLocal workspace without exposing every extension tool to ChatGPT.", {});
  remote("mcp_search_tools", "Search installed MCP tools", "Search the local MCP extension catalog and return only the most relevant tools.", { query: z.string().default(""), limit: z.number().int().min(1).max(50).default(8), server: z.string().optional(), refresh: z.boolean().default(false) });
  remote("mcp_tool_info", "Inspect MCP tool", "Get one installed MCP tool's exact schema before calling it.", { server: z.string().min(1), tool: z.string().min(1) });
  remote("mcp_call", "Call installed MCP tool", "Call one tool from an installed MCP extension. The local runtime approval policy remains authoritative.", { server: z.string().min(1), tool: z.string().min(1), arguments: z.record(z.unknown()).default({}) });
  return server;
}

async function revokeAndDisconnect(userId: string, credentialId: string) {
  const revoked = await deviceStore.revoke(userId, credentialId);
  if (!revoked) return false;
  for (const client of clients.values()) if (client.userId === userId && client.credentialId === credentialId) client.ws.close(4403, "device revoked");
  await cloudStore.audit(userId, "device.revoked", { credentialId });
  return true;
}

function deviceAuthFromRequest(req: Request) {
  const credentialId = String(req.headers["x-codelocal-credential-id"] ?? "");
  const authorization = String(req.headers.authorization ?? "");
  const secret = authorization.startsWith("Device ") ? authorization.slice(7) : "";
  return { credentialId, secret };
}

const app = express();
app.disable("x-powered-by");
app.use(express.json({ limit: "12mb" }));
app.use((req, res, next) => {
  const startedAt = Date.now();
  res.on("finish", () => { if (req.path !== "/health") log("info", "http.request", { method: req.method, path: req.path, status: res.statusCode, durationMs: Date.now() - startedAt, mcpSessionId: req.headers["mcp-session-id"] ?? null }); });
  next();
});
app.use(webAuthRouter);
app.use(oauthRouter);
app.use(createDashboardRouter({ onDeviceRevoked: async (userId, credentialId) => { await revokeAndDisconnect(userId, credentialId); } }));

app.get("/", async (req, res) => {
  if (await getWebIdentity(req)) { res.redirect(302, "/dashboard"); return; }
  res.type("html").send(authPage({
    title: "Your local development runtime for ChatGPT",
    subtitle: "Pair a machine, open a project with codelocal ., and let ChatGPT work through explicit MCP tools while your source code and secrets remain local.",
    body: `<div class="actions"><a class="btn primary" href="/register">Create account</a><a class="btn" href="/login">Sign in</a></div><div class="divider"></div><div class="label">One CodeLocal connection can route code intelligence, terminal, browser/simulator tools and installed MCP extensions without exposing hundreds of extension schemas at once.</div>`,
  }));
});

app.post("/pair/start", async (req, res) => {
  const deviceId = String(req.body?.deviceId ?? "").trim();
  const deviceName = String(req.body?.deviceName ?? deviceId).trim().slice(0, 120);
  if (!deviceId || deviceId.length > 200) { res.status(400).json({ error: "deviceId_required" }); return; }
  const pairing = await deviceStore.startPairing(deviceId, deviceName);
  const base = (process.env.PUBLIC_BASE_URL ?? `${req.protocol}://${req.get("host")}`).replace(/\/$/, "");
  res.json({ pairingId: pairing.pairingId, code: pairing.code, expiresAt: pairing.expiresAt, approveUrl: `${base}/pair/approve?pairingId=${encodeURIComponent(pairing.pairingId)}` });
});
app.get("/pair/approve", async (req, res) => {
  const pairingId = String(req.query.pairingId ?? "");
  const pairing = await deviceStore.getPairing(pairingId);
  if (!pairing || pairing.expiresAt <= Date.now() || pairing.claimedAt) { res.status(404).send("Pairing request not found or expired."); return; }
  const me = await getWebIdentity(req);
  if (!me) { res.redirect(302, `/login?next=${encodeURIComponent(req.originalUrl)}`); return; }
  res.type("html").send(authPage({
    title: "Approve device",
    subtitle: `Pair ${pairing.deviceName} with ${me.user.email}.`,
    body: `<div class="card" style="box-shadow:none;padding:16px;margin-bottom:16px"><div class="label">Pairing code shown in your terminal</div><div class="metric mono">${escapeHtml(pairing.code)}</div><div class="row-meta">Device ID: ${escapeHtml(pairing.deviceId)}</div></div><form class="form" method="post" action="/pair/approve"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}"><input type="hidden" name="pairingId" value="${escapeHtml(pairingId)}"><div class="field"><label>Confirm pairing code</label><input class="input mono" name="code" inputmode="numeric" pattern="[0-9]{6}" required></div><button class="btn primary" type="submit">Approve this device</button></form>`,
  }));
});
app.post("/pair/approve", express.urlencoded({ extended: false }), requireWebUser, async (req, res) => {
  if (!verifyCsrf(req)) { res.status(403).send("Invalid security token."); return; }
  const me = res.locals.webIdentity;
  const pairing = await deviceStore.approvePairing(String(req.body?.pairingId ?? ""), String(req.body?.code ?? ""), me.user.id);
  if (!pairing) { res.status(400).send("Invalid or expired pairing request/code."); return; }
  await cloudStore.audit(me.user.id, "device.pairing_approved", { deviceId: pairing.deviceId, deviceName: pairing.deviceName }, pairing.deviceId);
  res.type("html").send(authPage({ title: "Device approved", subtitle: "Return to your terminal. CodeLocal will claim its device credential automatically.", body: `<a class="btn primary" href="/dashboard/devices">View devices</a>` }));
});
app.post("/pair/claim", async (req, res) => {
  const credential = await deviceStore.claimPairing(String(req.body?.pairingId ?? ""), String(req.body?.code ?? ""));
  if (!credential) { res.status(400).json({ error: "pairing_not_approved_or_expired" }); return; }
  await cloudStore.audit(credential.userId, "device.paired", { credentialId: credential.credentialId, deviceName: credential.deviceName }, credential.deviceId);
  const { userId: _userId, ...publicCredential } = credential;
  res.json(publicCredential);
});

app.post("/api/client/mcp-sync", async (req, res) => {
  const { credentialId, secret } = deviceAuthFromRequest(req);
  const device = credentialId && secret ? await deviceStore.authenticate(credentialId, secret) : null;
  if (!device) { res.status(401).json({ error: "device_auth_failed" }); return; }
  const workspaceId = String(req.body?.workspaceId ?? "").trim();
  const installations = await cloudStore.listMcpInstallations(device.userId, workspaceId);
  res.json({ installations: installations.map(({ userId: _userId, ...item }) => item), syncedAt: Date.now() });
});

app.get("/health", (_req, res) => res.json({ ok: true, version: VERSION, protocolVersion: PROTOCOL_VERSION, cloud: true, onlineWorkspaces: clients.size, pendingToolCalls: pending.size }));
app.get("/api/status", async (req, res) => {
  const me = await getWebIdentity(req);
  if (!me) { res.status(401).json({ error: "unauthorized" }); return; }
  res.json({ user: { id: me.user.id, email: me.user.email }, workspaces: await cloudStore.listWorkspaces(me.user.id), devices: (await deviceStore.listDevices(me.user.id)).map((d) => ({ ...d })) });
});

app.use("/mcp", requireMcpAuth);
app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const userId = String(res.locals.oauth?.sub ?? "");
    if (!userId) { res.status(401).json({ error: "unauthorized" }); return; }
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) {
      const record = transports[sessionId];
      if (record.userId !== userId) { res.status(403).json({ error: "session_user_mismatch" }); return; }
      transport = record.transport;
    } else if (!sessionId && isInitializeRequest(req.body)) {
      transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(), enableJsonResponse: true,
        onsessioninitialized: (id) => { transports[id] = { transport, userId }; log("info", "mcp.session_open", { mcpSessionId: id, userId }); },
      });
      transport.onclose = () => { if (transport.sessionId) { delete transports[transport.sessionId]; log("info", "mcp.session_close", { mcpSessionId: transport.sessionId, userId }); } };
      await createMcpServer(userId).connect(transport);
    } else { res.status(400).json({ jsonrpc: "2.0", error: { code: -32000, message: "Invalid or missing MCP session." }, id: null }); return; }
    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    log("error", "mcp.request_failed", { error });
    if (!res.headersSent) res.status(500).json({ jsonrpc: "2.0", error: { code: -32603, message: "Internal error" }, id: null });
  }
});
app.get("/mcp", async (req: Request, res: Response) => {
  const userId = String(res.locals.oauth?.sub ?? "");
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  const record = sessionId ? transports[sessionId] : undefined;
  if (!record || record.userId !== userId) { res.status(400).send("Invalid MCP session"); return; }
  await record.transport.handleRequest(req, res);
});
app.delete("/mcp", async (req: Request, res: Response) => {
  const userId = String(res.locals.oauth?.sub ?? "");
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  const record = sessionId ? transports[sessionId] : undefined;
  if (!record || record.userId !== userId) { res.status(400).send("Invalid MCP session"); return; }
  await record.transport.handleRequest(req, res);
});

const httpServer = http.createServer(app);
const wss = new WebSocketServer({ server: httpServer, path: "/client" });
wss.on("connection", (ws) => {
  let authenticated = false;
  const authTimer = setTimeout(() => { if (!authenticated) ws.close(4401, "registration timeout"); }, 10_000);
  ws.on("message", async (raw) => {
    let msg: any;
    try { msg = JSON.parse(raw.toString()); } catch { ws.close(4400, "invalid json"); return; }
    if (!authenticated) {
      if (msg.type !== "register") { ws.close(4401, "register first"); return; }
      const version = typeof msg.protocolVersion === "number" ? msg.protocolVersion : 1;
      if (!protocolCompatible(version)) { ws.close(4406, `CLIENT_UPGRADE_REQUIRED:${MIN_PROTOCOL_VERSION}-${PROTOCOL_VERSION}`); return; }
      let device: any = null;
      if (msg.credentialId && msg.credentialSecret) device = await deviceStore.authenticate(String(msg.credentialId), String(msg.credentialSecret));
      else if (ALLOW_LEGACY_DEVICE_TOKEN && msg.token && equalSecret(String(msg.token), DEVICE_TOKEN)) {
        ws.close(4403, "Legacy DEVICE_TOKEN cannot establish a multi-tenant SaaS identity. Pair this device through the website."); return;
      }
      if (!device?.userId) { ws.close(4403, "AUTH_FAILED"); return; }
      const deviceId = String(msg.deviceId || device.deviceId || "device");
      if (deviceId !== device.deviceId) { ws.close(4403, "DEVICE_ID_MISMATCH"); return; }
      const workspaceId = String(msg.workspaceId || "workspace");
      const key = clientKey(device.userId, deviceId, workspaceId);
      const existing = clients.get(key);
      if (existing && existing.ws !== ws) existing.ws.close(4001, "replaced by newer connection");
      const record: ClientRecord = {
        key, userId: device.userId, deviceId, deviceName: String(msg.deviceName ?? device.deviceName ?? deviceId), workspaceId,
        workspaceName: String(msg.workspaceName ?? workspaceId), projectRoot: typeof msg.projectRoot === "string" ? msg.projectRoot : undefined,
        credentialId: device.credentialId, protocolVersion: version, capabilities: msg.capabilities ?? {}, ws, connectedAt: Date.now(), lastSeenAt: Date.now(),
      };
      clients.set(key, record); socketKeys.set(ws, key); authenticated = true; clearTimeout(authTimer);
      await cloudStore.upsertWorkspace({ userId: record.userId, deviceId: record.deviceId, workspaceId: record.workspaceId, workspaceName: record.workspaceName, projectRoot: record.projectRoot, protocolVersion: record.protocolVersion, capabilities: record.capabilities });
      ws.send(JSON.stringify({ type: "registered", protocolVersion: PROTOCOL_VERSION, serverCapabilities: { cancellation: true, idempotency: true, pairing: true, multiWorkspace: true, multiTenant: true, cloudMcpRegistry: true } }));
      log("info", "client.authenticated", { clientKey: key, userId: record.userId, protocolVersion: version, capabilities: record.capabilities });
      await cloudStore.audit(record.userId, "client.connected", { protocolVersion: version }, record.deviceId, record.workspaceId).catch(() => undefined);
      return;
    }
    const key = socketKeys.get(ws);
    const record = key ? clients.get(key) : undefined;
    if (record) record.lastSeenAt = Date.now();
    if (msg.type === "pong") {
      if (record) await cloudStore.touchWorkspace(record.userId, record.deviceId, record.workspaceId).catch(() => undefined);
      return;
    }
    if (msg.type !== "tool_result") return;
    const requestId = String(msg.requestId ?? msg.id ?? "");
    const wait = pending.get(requestId);
    if (!wait || !record || wait.userId !== record.userId) return;
    pending.delete(requestId); clearTimeout(wait.timer); wait.abortCleanup?.();
    const durationMs = Date.now() - wait.startedAt;
    if (msg.ok) { wait.resolve(msg.result); log("info", "tool.complete", { requestId, tool: wait.tool, clientKey: wait.clientKey, durationMs }); }
    else { const message = String(msg.errorMessage ?? msg.error ?? "Client tool failed"); const error = new Error(message); (error as any).code = msg.errorCode; wait.reject(error); log("error", "tool.failed", { requestId, tool: wait.tool, clientKey: wait.clientKey, durationMs, errorCode: msg.errorCode, error: message }); }
  });
  ws.on("close", () => {
    clearTimeout(authTimer);
    const key = socketKeys.get(ws); socketKeys.delete(ws);
    const record = key ? clients.get(key) : undefined;
    if (record?.ws === ws) {
      clients.delete(key!);
      void cloudStore.clearPresence(record.userId, record.deviceId, record.workspaceId).catch(() => undefined);
      void cloudStore.audit(record.userId, "client.disconnected", {}, record.deviceId, record.workspaceId).catch(() => undefined);
    }
    for (const [requestId, wait] of pending) {
      if (wait.clientKey !== key) continue;
      clearTimeout(wait.timer); wait.abortCleanup?.(); pending.delete(requestId); wait.reject(new Error("Client disconnected during tool call."));
    }
    if (key) log("warn", "client.disconnected", { clientKey: key });
  });
  ws.on("error", (error) => log("error", "client.socket_error", { error }));
});

const heartbeat = setInterval(() => {
  const now = Date.now();
  for (const client of clients.values()) {
    if (now - client.lastSeenAt > STALE_MS) { client.ws.terminate(); continue; }
    if (client.ws.readyState === client.ws.OPEN) client.ws.send(JSON.stringify({ type: "ping", protocolVersion: PROTOCOL_VERSION, ts: now }));
  }
}, HEARTBEAT_MS);
heartbeat.unref?.();

const shutdown = async () => {
  clearInterval(heartbeat);
  for (const client of clients.values()) client.ws.close(1001, "server shutdown");
  await cloudStore.close();
};
process.once("SIGTERM", () => { void shutdown().finally(() => process.exit(0)); });
process.once("SIGINT", () => { void shutdown().finally(() => process.exit(0)); });

httpServer.listen(PORT, HOST, () => log("info", "server.started", { host: HOST, port: PORT, version: VERSION, protocolVersion: PROTOCOL_VERSION, cloud: true, legacyDeviceTokenEnabled: ALLOW_LEGACY_DEVICE_TOKEN }));
