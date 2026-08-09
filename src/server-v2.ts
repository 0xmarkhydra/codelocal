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
import { DeviceStore } from "./device-store.js";
import { audit } from "./audit.js";
import { bridgeMcpToolResult } from "./mcp-bridge.js";

const VERSION = "1.5.0-beta.1";
const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN ?? "";
const MCP_USER_PASSWORD = process.env.MCP_USER_PASSWORD ?? "";
const TOOL_TIMEOUT_MS = Number(process.env.TOOL_TIMEOUT_MS ?? 180_000);
const HEARTBEAT_MS = Number(process.env.CODELOCAL_HEARTBEAT_MS ?? 20_000);
const STALE_MS = Number(process.env.CODELOCAL_STALE_MS ?? 70_000);
const ALLOW_LEGACY_DEVICE_TOKEN = process.env.ALLOW_LEGACY_DEVICE_TOKEN !== "0" && !!DEVICE_TOKEN;
const deviceStore = new DeviceStore();

type ClientCapabilities = {
  filesystem?: boolean; git?: boolean; shell?: boolean; pty?: boolean; sandbox?: string; semanticProviders?: string[]; idempotency?: boolean; cancellation?: boolean; approvals?: boolean; mcpHub?: boolean;
};

type ClientRecord = {
  key: string;
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
  abortCleanup?: () => void;
};

const clients = new Map<string, ClientRecord>();
const socketKeys = new Map<WebSocket, string>();
const pending = new Map<string, Pending>();
const transports: Record<string, StreamableHTTPServerTransport> = {};

function clientKey(deviceId: string, workspaceId: string) { return `${deviceId}::${workspaceId}`; }
function textResult(value: unknown) { return { content: [{ type: "text" as const, text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }] }; }
function availableClients() { return [...clients.values()].filter((c) => c.ws.readyState === c.ws.OPEN); }
function equalSecret(a: string, b: string) { const aa = Buffer.from(a); const bb = Buffer.from(b); return aa.length === bb.length && timingSafeEqual(aa, bb); }

function resolveClient(selectedKey?: string | null) {
  if (selectedKey) {
    const client = clients.get(selectedKey);
    if (!client || client.ws.readyState !== client.ws.OPEN) throw new Error(`Selected workspace is offline: ${selectedKey}`);
    return client;
  }
  const online = availableClients();
  if (online.length === 1) return online[0];
  if (!online.length) throw new Error("No local CodeLocal client is online.");
  throw new Error("Multiple workspaces are online. Call list_workspaces then select_workspace first.");
}

async function callClient(tool: string, args: unknown, selectedKey: string | null, options: { signal?: AbortSignal; sessionId?: string } = {}) {
  const client = resolveClient(selectedKey);
  if (tool.startsWith("mcp_") && !client.capabilities.mcpHub) throw new Error("Selected CodeLocal client does not support MCP Hub. Upgrade the local CLI/runtime.");
  const requestId = randomUUID();
  const startedAt = Date.now();
  const idempotencyKey = isSideEffectingTool(tool) ? requestId : undefined;
  log("info", "tool.dispatch", { requestId, tool, clientKey: client.key, args: summarizeToolArgs(tool, args), pending: pending.size });
  await audit({ event: "gateway.tool.dispatch", requestId, workspaceKey: client.key, tool });

  const result = new Promise<unknown>((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(requestId);
      if (client.capabilities.cancellation) client.ws.send(JSON.stringify({ type: "tool_cancel", protocolVersion: PROTOCOL_VERSION, requestId, reason: "gateway timeout" }));
      reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`));
    }, TOOL_TIMEOUT_MS);
    const entry: Pending = { resolve, reject, timer, tool, startedAt, clientKey: client.key };
    if (options.signal) {
      const onAbort = () => {
        if (!pending.has(requestId)) return;
        client.ws.send(JSON.stringify({ type: "tool_cancel", protocolVersion: PROTOCOL_VERSION, requestId, reason: "MCP request cancelled" }));
        clearTimeout(timer);
        pending.delete(requestId);
        reject(new Error("Tool request cancelled."));
      };
      options.signal.addEventListener("abort", onAbort, { once: true });
      entry.abortCleanup = () => options.signal?.removeEventListener("abort", onAbort);
    }
    pending.set(requestId, entry);
  });

  client.ws.send(JSON.stringify({
    type: "tool_call",
    protocolVersion: PROTOCOL_VERSION,
    requestId,
    id: requestId,
    sessionId: options.sessionId,
    workspaceKey: client.key,
    tool,
    args,
    idempotencyKey,
    deadline: Date.now() + TOOL_TIMEOUT_MS,
  }));
  return result;
}

function remoteSchema(server: McpServer, getSelected: () => string | null, name: string, title: string, description: string, inputSchema: Record<string, any>) {
  server.registerTool(name, { title, description, inputSchema }, async (args: any, extra: any) => {
    const result = await callClient(name, args, getSelected(), { signal: extra?.signal, sessionId: extra?.sessionId });
    return textResult(result);
  });
}

function createMcpServer() {
  const server = new McpServer({ name: "codelocal", version: VERSION });
  let selectedKey: string | null = null;
  const selected = () => selectedKey;
  const localTool = (name: string, title: string, description: string, inputSchema: Record<string, any>, handler: (args: any) => unknown | Promise<unknown>) => {
    server.registerTool(name, { title, description, inputSchema }, async (args: any) => textResult(await handler(args)));
  };

  localTool("list_devices", "List online devices", "List online CodeLocal devices and workspaces.", {}, async () => {
    const grouped = new Map<string, any[]>();
    for (const c of availableClients()) {
      const list = grouped.get(c.deviceId) ?? [];
      list.push({ workspaceId: c.workspaceId, workspaceName: c.workspaceName, key: c.key, projectRoot: c.projectRoot ?? null, protocolVersion: c.protocolVersion, capabilities: c.capabilities, lastSeenAt: c.lastSeenAt });
      grouped.set(c.deviceId, list);
    }
    return { devices: [...grouped.entries()].map(([deviceId, workspaces]) => ({ deviceId, workspaces })) };
  });
  localTool("list_device_identities", "List paired devices", "List durable paired device identities without secrets.", {}, () => deviceStore.listDevices());
  localTool("revoke_device", "Revoke device", "Revoke a paired device credential. This prevents future reconnects.", { credentialId: z.string().min(1) }, async (args) => ({ revoked: await deviceStore.revoke(args.credentialId) }));
  localTool("rename_device", "Rename device", "Rename a durable paired device identity.", { credentialId: z.string().min(1), deviceName: z.string().min(1).max(120) }, async (args) => ({ renamed: await deviceStore.rename(args.credentialId, args.deviceName) }));
  localTool("list_workspaces", "List workspaces", "List online workspaces and current selection.", {}, async () => ({ selectedWorkspace: selectedKey, workspaces: availableClients().map((c) => ({ key: c.key, deviceId: c.deviceId, deviceName: c.deviceName, workspaceId: c.workspaceId, workspaceName: c.workspaceName, projectRoot: c.projectRoot ?? null, capabilities: c.capabilities })) }));
  localTool("select_workspace", "Select workspace", "Select the workspace used by subsequent coding tools in this MCP session.", { key: z.string().min(1) }, async (args) => { const c = clients.get(args.key); if (!c || c.ws.readyState !== c.ws.OPEN) throw new Error(`Workspace unavailable: ${args.key}`); selectedKey = c.key; return { selected: c.key, deviceId: c.deviceId, workspaceId: c.workspaceId, workspaceName: c.workspaceName }; });
  localTool("workspace_info", "Workspace info", "Show the currently selected CodeLocal workspace.", {}, async () => { const c = resolveClient(selectedKey); return { selected: c.key, deviceId: c.deviceId, deviceName: c.deviceName, workspaceId: c.workspaceId, workspaceName: c.workspaceName, projectRoot: c.projectRoot ?? null, protocolVersion: c.protocolVersion, capabilities: c.capabilities, lastSeenAt: c.lastSeenAt }; });

  const remote = (name: string, title: string, description: string, schema: Record<string, any>) => remoteSchema(server, selected, name, title, description, schema);
  remote("project_info", "Project info", "Inspect workspace capabilities, project map, semantic providers, sandbox and instructions. Call first.", {});
  remote("project_map", "Project map", "Return cached compact project structure, languages, frameworks, commands and roots.", { force: z.boolean().default(false) });
  remote("context_for_task", "Context for task", "Select likely relevant symbols/files for a task hint before broad repository scans.", { taskHint: z.string().min(1), limit: z.number().int().min(1).max(100).default(30) });

  remote("mcp_list", "List installed MCPs", "List MCP extensions installed locally for the selected workspace. This does not expose every extension tool to ChatGPT.", {});
  remote("mcp_search_tools", "Search installed MCP tools", "Search the local cached MCP tool catalog by task intent. Use this instead of guessing extension tool names. Returns only the best matches, not the full tool universe.", { query: z.string().min(1), limit: z.number().int().min(1).max(50).default(8), server: z.string().optional(), refresh: z.boolean().default(false) });
  remote("mcp_tool_info", "Inspect installed MCP tool", "Get the exact input schema and metadata for one installed MCP tool before calling it.", { server: z.string().min(1), tool: z.string().min(1) });
  server.registerTool("mcp_call", {
    title: "Call installed MCP tool",
    description: "Call one tool from an MCP extension installed in CodeLocal. Prefer mcp_search_tools then mcp_tool_info first. External MCP execution is approval-gated locally and its individual tools are not exposed directly to ChatGPT.",
    inputSchema: { server: z.string().min(1), tool: z.string().min(1), arguments: z.record(z.unknown()).default({}) },
  }, async (args: any, extra: any) => {
    const result = await callClient("mcp_call", args, selected(), { signal: extra?.signal, sessionId: extra?.sessionId });
    const bridged = bridgeMcpToolResult(result);
    return (bridged ?? textResult(result)) as any;
  });

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
  return server;
}

const app = express();
app.use(express.json({ limit: "12mb" }));
app.use((req, res, next) => { const startedAt = Date.now(); res.on("finish", () => { if (req.path !== "/health") log("info", "http.request", { method: req.method, path: req.path, status: res.statusCode, durationMs: Date.now() - startedAt, mcpSessionId: req.headers["mcp-session-id"] ?? null }); }); next(); });
app.use(oauthRouter);

app.post("/pair/start", async (req, res) => {
  const deviceId = String(req.body?.deviceId ?? "").trim();
  const deviceName = String(req.body?.deviceName ?? deviceId).trim();
  if (!deviceId) { res.status(400).json({ error: "deviceId_required" }); return; }
  const pairing = await deviceStore.startPairing(deviceId, deviceName);
  const base = (process.env.PUBLIC_BASE_URL ?? `${req.protocol}://${req.get("host")}`).replace(/\/$/, "");
  res.json({ pairingId: pairing.pairingId, code: pairing.code, expiresAt: pairing.expiresAt, approveUrl: `${base}/pair/approve?pairingId=${encodeURIComponent(pairing.pairingId)}` });
});
app.get("/pair/approve", async (req, res) => {
  const pairingId = String(req.query.pairingId ?? "");
  const pairing = await deviceStore.getPairing(pairingId);
  if (!pairing || pairing.expiresAt <= Date.now()) { res.status(404).send("Pairing request not found or expired."); return; }
  res.type("html").send(`<!doctype html><html><body style="font-family:system-ui;max-width:480px;margin:60px auto;padding:20px"><h2>Pair CodeLocal device</h2><p>Device: <b>${pairing.deviceName.replace(/[<>&]/g, "")}</b></p><form method="post" action="/pair/approve"><input type="hidden" name="pairingId" value="${pairingId}"><label>Pairing code</label><input name="code" inputmode="numeric" required style="display:block;width:100%;padding:10px;margin:8px 0 16px"><label>CodeLocal passphrase</label><input name="password" type="password" required style="display:block;width:100%;padding:10px;margin:8px 0 16px"><button style="padding:10px 18px">Approve device</button></form></body></html>`);
});
app.post("/pair/approve", express.urlencoded({ extended: false }), async (req, res) => {
  const pairingId = String(req.body?.pairingId ?? ""); const code = String(req.body?.code ?? ""); const password = String(req.body?.password ?? "");
  if (!MCP_USER_PASSWORD || !equalSecret(password, MCP_USER_PASSWORD)) { res.status(401).send("Invalid passphrase."); return; }
  const pairing = await deviceStore.approvePairing(pairingId, code);
  if (!pairing) { res.status(400).send("Invalid or expired pairing request/code."); return; }
  res.send("Device approved. Return to the CodeLocal terminal; it can now claim its credential.");
});
app.post("/pair/claim", async (req, res) => {
  const credential = await deviceStore.claimPairing(String(req.body?.pairingId ?? ""), String(req.body?.code ?? ""));
  if (!credential) { res.status(400).json({ error: "pairing_not_approved_or_expired" }); return; }
  res.json(credential);
});
app.post("/devices/rotate", async (req, res) => {
  const password = String(req.body?.password ?? ""); if (!MCP_USER_PASSWORD || !equalSecret(password, MCP_USER_PASSWORD)) { res.status(401).json({ error: "unauthorized" }); return; }
  const rotated = await deviceStore.rotate(String(req.body?.credentialId ?? "")); if (!rotated) { res.status(404).json({ error: "device_not_found" }); return; } res.json(rotated);
});
app.post("/devices/revoke", async (req, res) => {
  const password = String(req.body?.password ?? ""); if (!MCP_USER_PASSWORD || !equalSecret(password, MCP_USER_PASSWORD)) { res.status(401).json({ error: "unauthorized" }); return; }
  res.json({ revoked: await deviceStore.revoke(String(req.body?.credentialId ?? "")) });
});

app.get("/", (_req, res) => res.json({ name: "codelocal", version: VERSION, protocolVersion: PROTOCOL_VERSION, minProtocolVersion: MIN_PROTOCOL_VERSION, status: "ok", mcp: "/mcp", websocket: "/client", oauth: true, pairing: true, mcpHub: true, onlineWorkspaces: availableClients().length, pendingToolCalls: pending.size, legacyDeviceTokenEnabled: ALLOW_LEGACY_DEVICE_TOKEN }));
app.get("/health", (_req, res) => res.json({ ok: true, version: VERSION, protocolVersion: PROTOCOL_VERSION, onlineWorkspaces: availableClients().length, pendingToolCalls: pending.size }));
app.use("/mcp", requireMcpAuth);

app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) transport = transports[sessionId];
    else if (!sessionId && isInitializeRequest(req.body)) {
      transport = new StreamableHTTPServerTransport({ sessionIdGenerator: () => randomUUID(), enableJsonResponse: true, onsessioninitialized: (id) => { transports[id] = transport; log("info", "mcp.session_open", { mcpSessionId: id }); } });
      transport.onclose = () => { if (transport.sessionId) { delete transports[transport.sessionId]; log("info", "mcp.session_close", { mcpSessionId: transport.sessionId }); } };
      await createMcpServer().connect(transport);
    } else { res.status(400).json({ jsonrpc: "2.0", error: { code: -32000, message: "Invalid or missing MCP session." }, id: null }); return; }
    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    log("error", "mcp.request_failed", { error });
    if (!res.headersSent) res.status(500).json({ jsonrpc: "2.0", error: { code: -32603, message: "Internal error" }, id: null });
  }
});
app.get("/mcp", async (req: Request, res: Response) => {
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  if (!sessionId || !transports[sessionId]) { res.status(400).send("Invalid MCP session"); return; }
  await transports[sessionId].handleRequest(req, res);
});
app.delete("/mcp", async (req: Request, res: Response) => {
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  if (!sessionId || !transports[sessionId]) { res.status(400).send("Invalid MCP session"); return; }
  await transports[sessionId].handleRequest(req, res);
});

const httpServer = http.createServer(app);
const wss = new WebSocketServer({ server: httpServer, path: "/client" });
wss.on("connection", (ws) => {
  let authenticated = false;
  const authTimer = setTimeout(() => { if (!authenticated) ws.close(4401, "registration timeout"); }, 10_000);
  ws.on("message", async (raw) => {
    let msg: any; try { msg = JSON.parse(raw.toString()); } catch { ws.close(4400, "invalid json"); return; }
    if (!authenticated) {
      if (msg.type !== "register") { ws.close(4401, "register first"); return; }
      const version = typeof msg.protocolVersion === "number" ? msg.protocolVersion : 1;
      if (!protocolCompatible(version)) { ws.close(4406, `CLIENT_UPGRADE_REQUIRED:${MIN_PROTOCOL_VERSION}-${PROTOCOL_VERSION}`); return; }
      let identity: any = null;
      if (msg.credentialId && msg.credentialSecret) identity = await deviceStore.authenticate(String(msg.credentialId), String(msg.credentialSecret));
      else if (ALLOW_LEGACY_DEVICE_TOKEN && msg.token && equalSecret(String(msg.token), DEVICE_TOKEN)) identity = { credentialId: undefined, deviceId: String(msg.deviceId), deviceName: String(msg.deviceName ?? msg.deviceId) };
      if (!identity) { ws.close(4403, "AUTH_FAILED"); return; }
      const deviceId = String(msg.deviceId || identity.deviceId || "device");
      const workspaceId = String(msg.workspaceId || "workspace");
      const key = clientKey(deviceId, workspaceId);
      const existing = clients.get(key);
      if (existing && existing.ws !== ws) existing.ws.close(4001, "replaced by newer connection");
      const record: ClientRecord = { key, deviceId, deviceName: String(msg.deviceName ?? identity.deviceName ?? deviceId), workspaceId, workspaceName: String(msg.workspaceName ?? workspaceId), projectRoot: typeof msg.projectRoot === "string" ? msg.projectRoot : undefined, credentialId: identity.credentialId, protocolVersion: version, capabilities: msg.capabilities ?? {}, ws, connectedAt: Date.now(), lastSeenAt: Date.now() };
      clients.set(key, record); socketKeys.set(ws, key); authenticated = true; clearTimeout(authTimer);
      ws.send(JSON.stringify({ type: "registered", protocolVersion: PROTOCOL_VERSION, serverCapabilities: { cancellation: true, idempotency: true, pairing: true, multiWorkspace: true, mcpHub: true } }));
      log("info", "client.authenticated", { clientKey: key, protocolVersion: version, capabilities: record.capabilities });
      return;
    }
    const key = socketKeys.get(ws); const record = key ? clients.get(key) : undefined; if (record) record.lastSeenAt = Date.now();
    if (msg.type === "pong") return;
    if (msg.type !== "tool_result") return;
    const requestId = String(msg.requestId ?? msg.id ?? "");
    const wait = pending.get(requestId); if (!wait) return;
    pending.delete(requestId); clearTimeout(wait.timer); wait.abortCleanup?.();
    const durationMs = Date.now() - wait.startedAt;
    if (msg.ok) { wait.resolve(msg.result); log("info", "tool.complete", { requestId, tool: wait.tool, clientKey: wait.clientKey, durationMs }); }
    else { const message = String(msg.errorMessage ?? msg.error ?? "Client tool failed"); const error = new Error(message); (error as any).code = msg.errorCode; wait.reject(error); log("error", "tool.failed", { requestId, tool: wait.tool, clientKey: wait.clientKey, durationMs, errorCode: msg.errorCode, error: message }); }
  });
  ws.on("close", () => {
    clearTimeout(authTimer);
    const key = socketKeys.get(ws); socketKeys.delete(ws);
    if (key && clients.get(key)?.ws === ws) clients.delete(key);
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

httpServer.listen(PORT, HOST, () => log("info", "server.started", { host: HOST, port: PORT, version: VERSION, protocolVersion: PROTOCOL_VERSION, legacyDeviceTokenEnabled: ALLOW_LEGACY_DEVICE_TOKEN }));
