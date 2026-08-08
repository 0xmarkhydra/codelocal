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

const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const TOOL_TIMEOUT_MS = 120_000;

if (!DEVICE_TOKEN) {
  log("error", "server.missing_device_token");
  process.exit(1);
}

type Pending = {
  resolve: (value: unknown) => void;
  reject: (reason?: unknown) => void;
  timer: NodeJS.Timeout;
  tool: string;
  startedAt: number;
};

let clientSocket: WebSocket | null = null;
const pending = new Map<string, Pending>();

function textResult(value: unknown) {
  return {
    content: [{ type: "text" as const, text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }],
  };
}

async function callClient(tool: string, args: unknown) {
  if (!clientSocket || clientSocket.readyState !== clientSocket.OPEN) {
    log("warn", "tool.rejected_client_offline", { tool });
    throw new Error("Local client is offline.");
  }

  const id = randomUUID();
  const startedAt = Date.now();
  log("info", "tool.dispatch", { requestId: id, tool, args: summarizeToolArgs(tool, args), pending: pending.size });

  const result = new Promise<unknown>((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      log("error", "tool.timeout", { requestId: id, tool, durationMs: Date.now() - startedAt });
      reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`));
    }, TOOL_TIMEOUT_MS);
    pending.set(id, { resolve, reject, timer, tool, startedAt });
  });

  clientSocket.send(JSON.stringify({ type: "tool_call", id, tool, args }));
  return result;
}

function createMcpServer() {
  const server = new McpServer({ name: "codex-mcp-gateway", version: "0.4.1" });
  const tool = (name: string, title: string, description: string, inputSchema: Record<string, any>) =>
    server.registerTool(name, { title, description, inputSchema }, async (args) => textResult(await callClient(name, args)));

  tool("project_info", "Project info", "Inspect the connected workspace, repo instructions, toolchains, branch, shell policy and capabilities. Call this first on an unfamiliar project.", {});
  tool("read_instructions", "Read scoped repo instructions", "Read AGENTS.md instructions from project root down to a requested file or directory, plus supported root-level coding instructions. Use before editing files in a nested area.", { path: z.string().default(".") });
  tool("list_files", "List project files", "Explore the project tree while skipping generated dependency/build directories. Prefer this over guessing paths.", { path: z.string().default("."), maxDepth: z.number().int().min(0).max(10).default(4) });
  tool("read_file", "Read file", "Read one UTF-8 source/text file from the local workspace.", { path: z.string().min(1) });
  tool("read_files", "Read multiple files", "Read up to 50 related source/text files in one call. Prefer this after search_code when several files are relevant.", { paths: z.array(z.string().min(1)).min(1).max(50) });
  tool("search_code", "Search code", "Search the workspace with ripgrep when available. Use to locate symbols, errors, tests, routes and references before reading files.", { query: z.string().min(1), path: z.string().default("."), maxResults: z.number().int().min(1).max(1000).default(200), fixedStrings: z.boolean().default(false) });
  tool("write_file", "Write file", "Create or overwrite a UTF-8 file inside the workspace. Prefer apply_patch for precise coding edits.", { path: z.string().min(1), content: z.string() });
  tool("edit_file", "Exact text edit", "Replace exact text in a file. Useful for small unambiguous edits; prefer apply_patch for multi-file changes.", { path: z.string().min(1), oldText: z.string().min(1), newText: z.string(), replaceAll: z.boolean().default(false) });
  tool("apply_patch", "Apply unified diff", "Validate paths and apply a standard unified git diff. Best tool for precise multi-file coding changes.", { patch: z.string().min(1) });
  tool("git_status", "Git status", "Show concise branch and working-tree status before or after edits.", {});
  tool("git_diff", "Git diff", "Inspect uncommitted changes. Use after editing to verify exactly what changed.", { cached: z.boolean().default(false), path: z.string().optional() });
  tool("run_command", "Run local command", "Run a local shell command with cwd constrained to the workspace and a CodeLocal command policy that blocks obvious system/destructive escape commands by default. Returns a processId. Use for builds, tests, linters and package scripts.", { command: z.string().min(1), cwd: z.string().default("."), yieldMs: z.number().int().min(0).max(10000).default(1000), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  tool("process_poll", "Poll incremental process output", "Read only process output produced since a cursor. Pass the cursor returned by the previous call to avoid repeating logs.", { processId: z.string().min(1), cursor: z.number().int().min(0).optional() });
  tool("process_list", "List local processes", "List CodeLocal processes started by run_command, including command, cwd, status and elapsed time.", {});
  tool("process_write", "Write process stdin", "Write text to stdin of a running local process.", { processId: z.string().min(1), input: z.string() });
  tool("process_kill", "Stop process", "Stop a running local process with SIGTERM or SIGKILL.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });

  return server;
}

const app = express();
app.use(express.json({ limit: "6mb" }));
app.use((req, res, next) => {
  const startedAt = Date.now();
  res.on("finish", () => {
    if (req.path === "/health") return;
    log("info", "http.request", {
      method: req.method,
      path: req.path,
      status: res.statusCode,
      durationMs: Date.now() - startedAt,
      mcpSessionId: req.headers["mcp-session-id"] ?? null,
    });
  });
  next();
});
app.use(oauthRouter);

app.get("/", (_req, res) => {
  res.json({
    name: "codex-mcp",
    version: "0.4.1",
    status: "ok",
    mcp: "/mcp",
    websocket: "/client",
    oauth: true,
    clientOnline: !!clientSocket,
    pendingToolCalls: pending.size,
  });
});

app.get("/health", (_req, res) => {
  res.json({ ok: true, version: "0.4.1", oauth: true, clientOnline: !!clientSocket, pendingToolCalls: pending.size });
});

app.use("/mcp", requireMcpAuth);

const transports: Record<string, StreamableHTTPServerTransport> = {};

app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;

    if (sessionId && transports[sessionId]) {
      transport = transports[sessionId];
    } else if (!sessionId && isInitializeRequest(req.body)) {
      log("info", "mcp.initialize", { remoteIp: req.ip });
      transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(),
        enableJsonResponse: true,
        onsessioninitialized: (id) => {
          transports[id] = transport;
          log("info", "mcp.session_open", { mcpSessionId: id, sessions: Object.keys(transports).length });
        },
      });
      transport.onclose = () => {
        if (transport.sessionId) {
          delete transports[transport.sessionId];
          log("info", "mcp.session_close", { mcpSessionId: transport.sessionId, sessions: Object.keys(transports).length });
        }
      };
      await createMcpServer().connect(transport);
    } else {
      log("warn", "mcp.invalid_session", { mcpSessionId: sessionId ?? null });
      res.status(400).json({
        jsonrpc: "2.0",
        error: { code: -32000, message: "Invalid or missing MCP session." },
        id: null,
      });
      return;
    }

    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    log("error", "mcp.request_error", { error });
    if (!res.headersSent) {
      res.status(500).json({
        jsonrpc: "2.0",
        error: { code: -32603, message: error instanceof Error ? error.message : "Internal server error" },
        id: null,
      });
    }
  }
});

async function handleSessionRequest(req: Request, res: Response) {
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  if (!sessionId || !transports[sessionId]) {
    log("warn", "mcp.invalid_session", { method: req.method, mcpSessionId: sessionId ?? null });
    res.status(400).send("Invalid or missing MCP session ID.");
    return;
  }
  await transports[sessionId].handleRequest(req, res);
}

app.get("/mcp", handleSessionRequest);
app.delete("/mcp", handleSessionRequest);

const httpServer = http.createServer(app);
const wss = new WebSocketServer({ server: httpServer, path: "/client" });

wss.on("connection", (ws, req) => {
  let authenticated = false;
  log("info", "client.socket_open", { remoteAddress: req.socket.remoteAddress ?? null });

  ws.on("message", (raw) => {
    try {
      const msg = JSON.parse(raw.toString());
      if (!authenticated) {
        if (msg.type !== "register" || msg.token !== DEVICE_TOKEN) {
          log("warn", "client.auth_rejected", { remoteAddress: req.socket.remoteAddress ?? null });
          ws.close(1008, "Invalid token");
          return;
        }
        authenticated = true;
        if (clientSocket && clientSocket !== ws) {
          log("warn", "client.replaced_existing");
          clientSocket.close(1012, "Replaced by new client");
        }
        clientSocket = ws;
        ws.send(JSON.stringify({ type: "registered" }));
        log("info", "client.authenticated", { pending: pending.size });
        return;
      }

      if (msg.type === "tool_result" && typeof msg.id === "string") {
        const item = pending.get(msg.id);
        if (!item) {
          log("warn", "tool.result_unknown", { requestId: msg.id });
          return;
        }
        clearTimeout(item.timer);
        pending.delete(msg.id);
        const durationMs = Date.now() - item.startedAt;
        if (msg.ok) {
          log("info", "tool.complete", { requestId: msg.id, tool: item.tool, durationMs, pending: pending.size });
          item.resolve(msg.result);
        } else {
          log("error", "tool.failed", { requestId: msg.id, tool: item.tool, durationMs, error: msg.error ?? "Client tool failed", pending: pending.size });
          item.reject(new Error(msg.error ?? "Client tool failed"));
        }
      }
    } catch (error) {
      log("warn", "client.invalid_message", { error });
      ws.close(1003, "Invalid JSON");
    }
  });

  ws.on("close", (code, reason) => {
    if (clientSocket === ws) clientSocket = null;
    log("warn", "client.socket_close", { code, reason: reason.toString(), authenticated });
  });

  ws.on("error", (error) => log("error", "client.socket_error", { error }));
});

httpServer.listen(PORT, HOST, () => {
  log("info", "server.started", { version: "0.4.1", host: HOST, port: PORT, oauth: true });
});