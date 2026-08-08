import express, { type Request, type Response } from "express";
import http from "node:http";
import { randomUUID } from "node:crypto";
import { WebSocketServer, type WebSocket } from "ws";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { isInitializeRequest } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { oauthRouter, requireMcpAuth } from "./oauth.js";

const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const TOOL_TIMEOUT_MS = 120_000;

if (!DEVICE_TOKEN) { console.error("Missing DEVICE_TOKEN"); process.exit(1); }

type Pending = { resolve: (value: unknown) => void; reject: (reason?: unknown) => void; timer: NodeJS.Timeout };
let clientSocket: WebSocket | null = null;
const pending = new Map<string, Pending>();

function textResult(value: unknown) {
  return { content: [{ type: "text" as const, text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }] };
}
async function callClient(tool: string, args: unknown) {
  if (!clientSocket || clientSocket.readyState !== clientSocket.OPEN) throw new Error("Local client is offline.");
  const id = randomUUID();
  const result = new Promise<unknown>((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`)); }, TOOL_TIMEOUT_MS);
    pending.set(id, { resolve, reject, timer });
  });
  clientSocket.send(JSON.stringify({ type: "tool_call", id, tool, args }));
  return result;
}

function createMcpServer() {
  const server = new McpServer({ name: "codex-mcp-gateway", version: "0.3.1" });
  const tool = (name: string, title: string, description: string, inputSchema: Record<string, any>) =>
    server.registerTool(name, { title, description, inputSchema }, async (args) => textResult(await callClient(name, args)));

  tool("project_info", "Project info", "Inspect the connected local workspace, repository status, shell availability and client capabilities. Call this first when beginning work on an unfamiliar project.", {});
  tool("list_files", "List project files", "Explore the project tree. node_modules and .git are skipped. Prefer this over guessing paths.", { path: z.string().default("."), maxDepth: z.number().int().min(0).max(10).default(4) });
  tool("read_file", "Read file", "Read one UTF-8 source/text file from the local workspace.", { path: z.string().min(1) });
  tool("read_files", "Read multiple files", "Read up to 50 source/text files in one call. Prefer this after code search when several related files are needed.", { paths: z.array(z.string().min(1)).min(1).max(50) });
  tool("search_code", "Search code", "Search the workspace using ripgrep when available. Use this to locate symbols, errors, routes, tests, configuration and references before reading files.", { query: z.string().min(1), path: z.string().default("."), maxResults: z.number().int().min(1).max(1000).default(200), fixedStrings: z.boolean().default(false) });
  tool("write_file", "Write file", "Create or overwrite a UTF-8 file inside the workspace. Prefer apply_patch for surgical multi-file coding edits.", { path: z.string().min(1), content: z.string() });
  tool("edit_file", "Exact text edit", "Replace exact text in a file. Useful for small unambiguous edits; prefer apply_patch for larger code changes.", { path: z.string().min(1), oldText: z.string().min(1), newText: z.string(), replaceAll: z.boolean().default(false) });
  tool("apply_patch", "Apply unified diff", "Apply a standard unified git diff to the workspace after validating paths and running git apply --check. Best tool for precise multi-file code edits.", { patch: z.string().min(1) });
  tool("git_status", "Git status", "Show concise branch and working-tree status after edits or before changing code.", {});
  tool("git_diff", "Git diff", "Inspect uncommitted changes. Use after editing to verify exactly what changed.", { cached: z.boolean().default(false), path: z.string().optional() });
  tool("run_command", "Run local command", "Run a shell command with cwd constrained to the workspace. Returns quickly with a processId; if still running, use process_poll. Requires the local client to be started with CODELOCAL_ALLOW_SHELL=1. Use for builds, tests, linters, package scripts and developer tooling.", { command: z.string().min(1), cwd: z.string().default("."), yieldMs: z.number().int().min(0).max(10000).default(1000), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  tool("process_poll", "Poll process", "Get current output and exit status for a process returned by run_command. Poll long-running builds or dev servers instead of starting duplicates.", { processId: z.string().min(1) });
  tool("process_write", "Write process stdin", "Write text to stdin of a running local process.", { processId: z.string().min(1), input: z.string() });
  tool("process_kill", "Stop process", "Stop a running local process with SIGTERM or SIGKILL.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });
  return server;
}

const app = express();
app.use(express.json({ limit: "6mb" }));
app.use(oauthRouter);
app.get("/", (_req, res) => res.json({ name: "codex-mcp", version: "0.3.1", status: "ok", mcp: "/mcp", websocket: "/client", oauth: true, clientOnline: !!clientSocket }));
app.get("/health", (_req, res) => res.json({ ok: true, version: "0.3.1", oauth: true, clientOnline: !!clientSocket }));
app.use("/mcp", requireMcpAuth);

const transports: Record<string, StreamableHTTPServerTransport> = {};
app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) transport = transports[sessionId];
    else if (!sessionId && isInitializeRequest(req.body)) {
      transport = new StreamableHTTPServerTransport({ sessionIdGenerator: () => randomUUID(), enableJsonResponse: true, onsessioninitialized: (id) => { transports[id] = transport; } });
      transport.onclose = () => { if (transport.sessionId) delete transports[transport.sessionId]; };
      await createMcpServer().connect(transport);
    } else { res.status(400).json({ jsonrpc: "2.0", error: { code: -32000, message: "Invalid or missing MCP session." }, id: null }); return; }
    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    if (!res.headersSent) res.status(500).json({ jsonrpc: "2.0", error: { code: -32603, message: error instanceof Error ? error.message : "Internal server error" }, id: null });
  }
});
async function handleSessionRequest(req: Request, res: Response) {
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  if (!sessionId || !transports[sessionId]) return void res.status(400).send("Invalid or missing MCP session ID.");
  await transports[sessionId].handleRequest(req, res);
}
app.get("/mcp", handleSessionRequest);
app.delete("/mcp", handleSessionRequest);

const httpServer = http.createServer(app);
const wss = new WebSocketServer({ server: httpServer, path: "/client" });
wss.on("connection", (ws) => {
  let authenticated = false;
  ws.on("message", (raw) => {
    try {
      const msg = JSON.parse(raw.toString());
      if (!authenticated) {
        if (msg.type !== "register" || msg.token !== DEVICE_TOKEN) return ws.close(1008, "Invalid token");
        authenticated = true;
        if (clientSocket && clientSocket !== ws) clientSocket.close(1012, "Replaced by new client");
        clientSocket = ws; ws.send(JSON.stringify({ type: "registered" })); return;
      }
      if (msg.type === "tool_result" && typeof msg.id === "string") {
        const item = pending.get(msg.id); if (!item) return;
        clearTimeout(item.timer); pending.delete(msg.id);
        if (msg.ok) item.resolve(msg.result); else item.reject(new Error(msg.error ?? "Client tool failed"));
      }
    } catch { ws.close(1003, "Invalid JSON"); }
  });
  ws.on("close", () => { if (clientSocket === ws) clientSocket = null; });
});
httpServer.listen(PORT, HOST, () => console.log(`codex-mcp 0.3.1 listening on ${HOST}:${PORT}`));
