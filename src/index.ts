import express, { type Request, type Response } from "express";
import http from "node:http";
import { randomUUID } from "node:crypto";
import { WebSocketServer, type WebSocket } from "ws";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { isInitializeRequest } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";

const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const TOOL_TIMEOUT_MS = 30_000;

if (!DEVICE_TOKEN) {
  console.error("Missing DEVICE_TOKEN");
  process.exit(1);
}

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
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`));
    }, TOOL_TIMEOUT_MS);
    pending.set(id, { resolve, reject, timer });
  });
  clientSocket.send(JSON.stringify({ type: "tool_call", id, tool, args }));
  return result;
}

function createMcpServer() {
  const server = new McpServer({ name: "codex-mcp-gateway", version: "0.2.0" });

  server.registerTool("project_info", { title: "Project info", description: "Show the connected local project and client status.", inputSchema: {} }, async () => textResult(await callClient("project_info", {})));
  server.registerTool("list_files", { title: "List files", description: "List files inside the connected PROJECT_ROOT.", inputSchema: { path: z.string().default("."), maxDepth: z.number().int().min(0).max(10).default(4) } }, async (args) => textResult(await callClient("list_files", args)));
  server.registerTool("read_file", { title: "Read file", description: "Read a UTF-8 text file inside PROJECT_ROOT.", inputSchema: { path: z.string().min(1) } }, async (args) => textResult(await callClient("read_file", args)));
  server.registerTool("write_file", { title: "Write file", description: "Create or overwrite a UTF-8 text file inside PROJECT_ROOT.", inputSchema: { path: z.string().min(1), content: z.string() } }, async (args) => textResult(await callClient("write_file", args)));
  server.registerTool("edit_file", { title: "Edit file", description: "Replace exact text inside a UTF-8 file in PROJECT_ROOT.", inputSchema: { path: z.string().min(1), oldText: z.string().min(1), newText: z.string(), replaceAll: z.boolean().default(false) } }, async (args) => textResult(await callClient("edit_file", args)));
  return server;
}

const app = express();
app.use(express.json({ limit: "2mb" }));
app.get("/", (_req, res) => res.json({ name: "codex-mcp", status: "ok", mcp: "/mcp", websocket: "/client", clientOnline: !!clientSocket }));
app.get("/health", (_req, res) => res.json({ ok: true, clientOnline: !!clientSocket }));

const transports: Record<string, StreamableHTTPServerTransport> = {};
app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) {
      transport = transports[sessionId];
    } else if (!sessionId && isInitializeRequest(req.body)) {
      transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(),
        enableJsonResponse: true,
        onsessioninitialized: (id) => { transports[id] = transport; },
      });
      transport.onclose = () => { if (transport.sessionId) delete transports[transport.sessionId]; };
      await createMcpServer().connect(transport);
    } else {
      res.status(400).json({ jsonrpc: "2.0", error: { code: -32000, message: "Invalid or missing MCP session." }, id: null });
      return;
    }
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
        clientSocket = ws;
        ws.send(JSON.stringify({ type: "registered" }));
        return;
      }
      if (msg.type === "tool_result" && typeof msg.id === "string") {
        const item = pending.get(msg.id);
        if (!item) return;
        clearTimeout(item.timer);
        pending.delete(msg.id);
        if (msg.ok) item.resolve(msg.result); else item.reject(new Error(msg.error ?? "Client tool failed"));
      }
    } catch { ws.close(1003, "Invalid JSON"); }
  });
  ws.on("close", () => { if (clientSocket === ws) clientSocket = null; });
});

httpServer.listen(PORT, HOST, () => console.log(`codex-mcp listening on ${HOST}:${PORT}`));
