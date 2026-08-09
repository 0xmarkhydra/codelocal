import express, { type Request, type Response } from "express";
import http from "node:http";
import { randomUUID } from "node:crypto";
import { WebSocketServer, type WebSocket } from "ws";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { isInitializeRequest } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { oauthRouter, requireMcpAuth } from "./oauth.js";
import { log, summarizeToolArgs } from "./log.js";
import { VERSION } from "./version.js";
const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const TOOL_TIMEOUT_MS = Number(process.env.TOOL_TIMEOUT_MS ?? 180_000);
const HEARTBEAT_MS = 20_000;
if (!DEVICE_TOKEN) { log("error", "server.missing_device_token"); process.exit(1); }

type ClientRecord = { key: string; deviceId: string; workspaceId: string; workspaceName: string; projectRoot?: string; ws: WebSocket; connectedAt: number; lastSeenAt: number };
type Pending = { resolve: (v: unknown) => void; reject: (e?: unknown) => void; timer: NodeJS.Timeout; tool: string; startedAt: number; clientKey: string };
const clients = new Map<string, ClientRecord>();
const socketKeys = new Map<WebSocket, string>();
const pending = new Map<string, Pending>();

function clientKey(deviceId: string, workspaceId: string) { return `${deviceId}::${workspaceId}`; }
function textResult(value: unknown) { return { content: [{ type: "text" as const, text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }] }; }
function availableClients() { return [...clients.values()].filter((c) => c.ws.readyState === c.ws.OPEN); }
function resolveClient(selectedKey?: string | null) {
  if (selectedKey) { const c = clients.get(selectedKey); if (!c || c.ws.readyState !== c.ws.OPEN) throw new Error(`Selected workspace is offline: ${selectedKey}`); return c; }
  const online = availableClients();
  if (online.length === 1) return online[0];
  if (!online.length) throw new Error("No local CodeLocal client is online.");
  throw new Error("Multiple workspaces are online. Call list_workspaces then select_workspace first.");
}
async function callClient(tool: string, args: unknown, selectedKey?: string | null) {
  const client = resolveClient(selectedKey);
  const id = randomUUID(); const startedAt = Date.now();
  log("info", "tool.dispatch", { requestId: id, tool, clientKey: client.key, args: summarizeToolArgs(tool, args), pending: pending.size });
  const result = new Promise<unknown>((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); log("error", "tool.timeout", { requestId: id, tool, clientKey: client.key, durationMs: Date.now() - startedAt }); reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`)); }, TOOL_TIMEOUT_MS);
    pending.set(id, { resolve, reject, timer, tool, startedAt, clientKey: client.key });
  });
  client.ws.send(JSON.stringify({ type: "tool_call", id, tool, args }));
  return result;
}

function createMcpServer() {
  const server = new McpServer({ name: "codex-mcp-gateway", version: VERSION });
  let selectedKey: string | null = null;
  const localTool = (name: string, title: string, description: string, inputSchema: Record<string, any>, handler: (args: any) => unknown | Promise<unknown>) => server.registerTool(name, { title, description, inputSchema }, async (args) => textResult(await handler(args)));
  const remoteTool = (name: string, title: string, description: string, inputSchema: Record<string, any>) => server.registerTool(name, { title, description, inputSchema }, async (args) => textResult(await callClient(name, args, selectedKey)));

  localTool("list_devices", "List CodeLocal devices", "List online CodeLocal devices and their registered workspaces.", {}, async () => {
    const grouped = new Map<string, any[]>();
    for (const c of availableClients()) { const list = grouped.get(c.deviceId) ?? []; list.push({ workspaceId: c.workspaceId, workspaceName: c.workspaceName, key: c.key, projectRoot: c.projectRoot ?? null, connectedAt: c.connectedAt, lastSeenAt: c.lastSeenAt }); grouped.set(c.deviceId, list); }
    return { devices: [...grouped.entries()].map(([deviceId, workspaces]) => ({ deviceId, workspaces })) };
  });
  localTool("list_workspaces", "List CodeLocal workspaces", "List all online workspaces. Use when more than one client/workspace is connected.", {}, async () => ({ selectedWorkspace: selectedKey, workspaces: availableClients().map((c) => ({ key: c.key, deviceId: c.deviceId, workspaceId: c.workspaceId, workspaceName: c.workspaceName, projectRoot: c.projectRoot ?? null })) }));
  localTool("select_workspace", "Select workspace", "Select the workspace used by subsequent coding tools in this MCP session. The model should not switch silently.", { key: z.string().min(1) }, async (args) => { const c = clients.get(args.key); if (!c || c.ws.readyState !== c.ws.OPEN) throw new Error(`Workspace unavailable: ${args.key}`); selectedKey = c.key; return { selected: c.key, deviceId: c.deviceId, workspaceId: c.workspaceId, workspaceName: c.workspaceName }; });
  localTool("workspace_info", "Workspace routing info", "Show the currently selected CodeLocal workspace for this MCP session.", {}, async () => { const c = resolveClient(selectedKey); return { selected: c.key, deviceId: c.deviceId, workspaceId: c.workspaceId, workspaceName: c.workspaceName, projectRoot: c.projectRoot ?? null, lastSeenAt: c.lastSeenAt }; });

  remoteTool("project_info", "Project info", "Inspect the connected workspace, instructions, languages/frameworks, semantic index, branch, shell/approval policy and capabilities. Call this first.", {});
  remoteTool("read_instructions", "Read scoped repo instructions", "Read AGENTS.md and supported coding instructions applying to a file/directory.", { path: z.string().default(".") });
  remoteTool("list_files", "List files", "List project files with .gitignore-aware retrieval. Ignored paths are excluded by default; sensitive paths are never readable.", { path: z.string().default("."), maxDepth: z.number().int().min(0).max(20).default(4), includeIgnored: z.boolean().default(false) });
  remoteTool("file_info", "File metadata", "Return file size, mtime, hash, ignore status and type without reading source contents.", { path: z.string().min(1) });
  remoteTool("read_file", "Read file", "Read a targeted UTF-8 file. Ignored files can be targeted; sensitive files remain blocked.", { path: z.string().min(1) });
  remoteTool("read_file_range", "Read file range", "Read only a requested line range from a text file to reduce context use.", { path: z.string().min(1), startLine: z.number().int().min(1), endLine: z.number().int().min(1) });
  remoteTool("read_files", "Read multiple files", "Read up to 50 targeted files in one call.", { paths: z.array(z.string().min(1)).min(1).max(50) });
  remoteTool("search_code", "Search code", "Search source with gitignore-aware behavior. Set includeIgnored only for targeted dependency/generated investigations.", { query: z.string().min(1), path: z.string().default("."), maxResults: z.number().int().min(1).max(1000).default(200), fixedStrings: z.boolean().default(false), includeIgnored: z.boolean().default(false) });

  remoteTool("inspect_dependency", "Inspect dependency", "Inspect installed/declaration metadata for a dependency without scanning its full source tree.", { name: z.string().min(1) });
  remoteTool("read_dependency", "Read dependency file", "Read a targeted file/range inside an installed dependency.", { name: z.string().min(1), path: z.string().default("package.json"), startLine: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional() });
  remoteTool("search_dependency", "Search dependency", "Search only inside one installed dependency.", { name: z.string().min(1), query: z.string().min(1), maxResults: z.number().int().min(1).max(500).default(100), fixedStrings: z.boolean().default(false) });

  remoteTool("find_symbol", "Find symbols", "Find TypeScript/JavaScript declarations by symbol name/query using the local semantic index.", { query: z.string().default(""), limit: z.number().int().min(1).max(1000).default(200) });
  remoteTool("find_definition", "Find definition", "Locate definitions/declarations for a TypeScript/JavaScript symbol.", { name: z.string().min(1), limit: z.number().int().min(1).max(500).default(100) });
  remoteTool("find_references", "Find references", "Locate project references for a TypeScript/JavaScript symbol.", { name: z.string().min(1), limit: z.number().int().min(1).max(2000).default(500) });
  remoteTool("get_callers", "Get callers", "Find call sites that call a named function/method.", { name: z.string().min(1), limit: z.number().int().min(1).max(1000).default(300) });
  remoteTool("get_callees", "Get callees", "Find calls made inside a named function/method.", { name: z.string().min(1), limit: z.number().int().min(1).max(1000).default(300) });
  remoteTool("get_import_graph", "Get import graph", "Return lightweight project import/export graph edges.", { limit: z.number().int().min(1).max(10000).default(2000) });
  remoteTool("get_diagnostics", "Get diagnostics", "Return TypeScript compiler diagnostics from the semantic project model.", { limit: z.number().int().min(1).max(2000).default(500) });

  remoteTool("detect_test_commands", "Detect test/check commands", "Detect available project test, lint, typecheck, build and compiler commands.", {});
  remoteTool("find_related_tests", "Find related tests", "Find likely tests related to a source file.", { path: z.string().min(1) });
  remoteTool("run_affected_tests", "Run affected tests", "Run the detected primary test command after locating related tests. Shell approval policy still applies.", { path: z.string().min(1), timeoutMs: z.number().int().min(0).max(3_600_000).default(300000) });

  remoteTool("write_file", "Write file", "Create/overwrite a file. Pass expectedHash after a prior read to prevent stale overwrites.", { path: z.string().min(1), content: z.string(), expectedHash: z.string().optional() });
  remoteTool("edit_file", "Exact text edit", "Replace exact text, optionally requiring the file hash observed during the prior read.", { path: z.string().min(1), oldText: z.string().min(1), newText: z.string(), replaceAll: z.boolean().default(false), expectedHash: z.string().optional() });
  remoteTool("apply_patch", "Apply unified diff", "Validate paths then git-apply a unified patch. Sensitive/workspace-escape paths are blocked.", { patch: z.string().min(1) });

  remoteTool("git_status", "Git status", "Show branch and working-tree status.", {});
  remoteTool("git_diff", "Git diff", "Inspect uncommitted changes.", { cached: z.boolean().default(false), path: z.string().optional() });
  remoteTool("git_log", "Git log", "Read recent commit history, optionally for one path.", { limit: z.number().int().min(1).max(100).default(20), path: z.string().optional() });
  remoteTool("git_show", "Git show", "Show a commit/ref summary and patch/stat output.", { ref: z.string().default("HEAD") });
  remoteTool("git_blame", "Git blame", "Read blame data for a file or line range.", { path: z.string().min(1), startLine: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional() });
  remoteTool("git_file_history", "Git file history", "Read follow-renames history for one file.", { path: z.string().min(1), limit: z.number().int().min(1).max(100).default(30) });

  remoteTool("run_command", "Run local command", "Run a guarded local shell command. Risky package/network/Git/migration operations require local terminal approval; blocked system/credential operations remain denied unless explicitly unsafe mode is enabled.", { command: z.string().min(1), cwd: z.string().default("."), yieldMs: z.number().int().min(0).max(10000).default(1000), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remoteTool("process_poll", "Poll process", "Read incremental output from a CodeLocal process using cursors.", { processId: z.string().min(1), cursor: z.number().int().min(0).optional() });
  remoteTool("process_list", "List processes", "List CodeLocal-started processes and status.", {});
  remoteTool("process_write", "Write process stdin", "Write stdin to a running process.", { processId: z.string().min(1), input: z.string() });
  remoteTool("process_kill", "Stop process", "Stop a process with SIGTERM/SIGKILL.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });
  return server;
}

const app = express();
app.use(express.json({ limit: "12mb" }));
app.use((req, res, next) => { const startedAt = Date.now(); res.on("finish", () => { if (req.path !== "/health") log("info", "http.request", { method: req.method, path: req.path, status: res.statusCode, durationMs: Date.now() - startedAt, mcpSessionId: req.headers["mcp-session-id"] ?? null }); }); next(); });
app.use(oauthRouter);
app.get("/", (_req, res) => res.json({ name: "codex-mcp", version: VERSION, status: "ok", mcp: "/mcp", websocket: "/client", oauth: true, onlineWorkspaces: availableClients().length, pendingToolCalls: pending.size }));
app.get("/health", (_req, res) => res.json({ ok: true, version: VERSION, oauth: true, onlineWorkspaces: availableClients().length, pendingToolCalls: pending.size }));
app.use("/mcp", requireMcpAuth);

const transports: Record<string, StreamableHTTPServerTransport> = {};
app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) transport = transports[sessionId];
    else if (!sessionId && isInitializeRequest(req.body)) {
      log("info", "mcp.initialize", { remoteIp: req.ip });
      transport = new StreamableHTTPServerTransport({ sessionIdGenerator: () => randomUUID(), enableJsonResponse: true, onsessioninitialized: (id) => { transports[id] = transport; log("info", "mcp.session_open", { mcpSessionId: id, sessions: Object.keys(transports).length }); } });
      transport.onclose = () => { if (transport.sessionId) { delete transports[transport.sessionId]; log("info", "mcp.session_close", { mcpSessionId: transport.sessionId }); } };
      await createMcpServer().connect(transport);
    } else { res.status(400).json({ jsonrpc: "2.0", error: { code: -32000, message: "Invalid or missing MCP session." }, id: null }); return; }
    await transport.handleRequest(req, res, req.body);
  } catch (error) { log("error", "mcp.request_error", { error }); if (!res.headersSent) res.status(500).json({ jsonrpc: "2.0", error: { code: -32603, message: error instanceof Error ? error.message : "Internal server error" }, id: null }); }
});
async function handleSessionRequest(req: Request, res: Response) { const id = req.headers["mcp-session-id"] as string | undefined; if (!id || !transports[id]) { res.status(400).send("Invalid or missing MCP session ID."); return; } await transports[id].handleRequest(req, res); }
app.get("/mcp", handleSessionRequest); app.delete("/mcp", handleSessionRequest);

const httpServer = http.createServer(app);
const wss = new WebSocketServer({ server: httpServer, path: "/client" });
wss.on("connection", (ws, req) => {
  let authenticated = false; log("info", "client.socket_open", { remoteAddress: req.socket.remoteAddress ?? null });
  ws.on("message", (raw) => {
    try {
      const msg = JSON.parse(raw.toString());
      if (!authenticated) {
        if (msg.type !== "register" || msg.token !== DEVICE_TOKEN) { log("warn", "client.auth_rejected"); ws.close(1008, "Invalid token"); return; }
        const deviceId = String(msg.deviceId || "device"); const workspaceId = String(msg.workspaceId || "workspace"); const key = clientKey(deviceId, workspaceId);
        const existing = clients.get(key); if (existing && existing.ws !== ws) existing.ws.close(1012, "Workspace reconnected");
        const record: ClientRecord = { key, deviceId, workspaceId, workspaceName: String(msg.workspaceName || workspaceId), projectRoot: typeof msg.projectRoot === "string" ? msg.projectRoot : undefined, ws, connectedAt: Date.now(), lastSeenAt: Date.now() };
        clients.set(key, record); socketKeys.set(ws, key); authenticated = true; ws.send(JSON.stringify({ type: "registered", key })); log("info", "client.authenticated", { clientKey: key, deviceId, workspaceId, onlineWorkspaces: availableClients().length }); return;
      }
      const key = socketKeys.get(ws); if (key) { const c = clients.get(key); if (c) c.lastSeenAt = Date.now(); }
      if (msg.type === "pong") return;
      if (msg.type === "tool_result" && typeof msg.id === "string") {
        const item = pending.get(msg.id); if (!item) { log("warn", "tool.result_unknown", { requestId: msg.id }); return; }
        clearTimeout(item.timer); pending.delete(msg.id); const durationMs = Date.now() - item.startedAt;
        if (msg.ok) { log("info", "tool.complete", { requestId: msg.id, tool: item.tool, clientKey: item.clientKey, durationMs }); item.resolve(msg.result); }
        else { log("error", "tool.failed", { requestId: msg.id, tool: item.tool, clientKey: item.clientKey, durationMs, error: msg.error ?? "Client tool failed" }); item.reject(new Error(msg.error ?? "Client tool failed")); }
      }
    } catch (error) { log("warn", "client.invalid_message", { error }); ws.close(1003, "Invalid JSON"); }
  });
  ws.on("close", (code, reason) => {
    const key = socketKeys.get(ws); socketKeys.delete(ws); if (key && clients.get(key)?.ws === ws) clients.delete(key);
    if (key) for (const [id, item] of pending) if (item.clientKey === key) { clearTimeout(item.timer); pending.delete(id); item.reject(new Error("Local client disconnected during tool call.")); }
    log("warn", "client.socket_close", { clientKey: key ?? null, code, reason: reason.toString(), onlineWorkspaces: availableClients().length });
  });
  ws.on("error", (error) => log("error", "client.socket_error", { error }));
});
setInterval(() => { const now = Date.now(); for (const c of clients.values()) { if (c.ws.readyState === c.ws.OPEN) { try { c.ws.send(JSON.stringify({ type: "ping", ts: now })); } catch {} } if (now - c.lastSeenAt > HEARTBEAT_MS * 4) { log("warn", "client.heartbeat_stale", { clientKey: c.key, ageMs: now - c.lastSeenAt }); } } }, HEARTBEAT_MS).unref();

httpServer.listen(PORT, HOST, () => log("info", "server.started", { version: VERSION, host: HOST, port: PORT, oauth: true, multiWorkspace: true }));
