import express, { type Request, type Response } from "express";
import { randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { isInitializeRequest } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";

const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "127.0.0.1";
const configuredRoot = process.env.PROJECT_ROOT;

if (!configuredRoot) {
  console.error("Missing PROJECT_ROOT. Example: PROJECT_ROOT=/Users/me/Projects/my-app npm start");
  process.exit(1);
}

const root = await fs.realpath(path.resolve(configuredRoot));
const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
const MAX_READ_BYTES = 1024 * 1024;
const MAX_LIST_ENTRIES = 5000;

function isInsideRoot(candidate: string): boolean {
  return candidate === root || candidate.startsWith(rootPrefix);
}

function lexicalPath(relativePath: string): string {
  if (path.isAbsolute(relativePath)) {
    throw new Error("Absolute paths are not allowed. Use a path relative to PROJECT_ROOT.");
  }

  const candidate = path.resolve(root, relativePath || ".");
  if (!isInsideRoot(candidate)) {
    throw new Error("Path escapes PROJECT_ROOT.");
  }
  return candidate;
}

async function safeExistingPath(relativePath: string): Promise<string> {
  const candidate = lexicalPath(relativePath);
  const real = await fs.realpath(candidate);
  if (!isInsideRoot(real)) {
    throw new Error("Resolved path escapes PROJECT_ROOT (possible symlink traversal).");
  }
  return real;
}

async function safeWritePath(relativePath: string): Promise<string> {
  const candidate = lexicalPath(relativePath);
  const parent = path.dirname(candidate);
  const realParent = await fs.realpath(parent);
  if (!isInsideRoot(realParent)) {
    throw new Error("Parent directory escapes PROJECT_ROOT (possible symlink traversal).");
  }

  try {
    const existingReal = await fs.realpath(candidate);
    if (!isInsideRoot(existingReal)) {
      throw new Error("Existing file escapes PROJECT_ROOT (possible symlink traversal).");
    }
    return existingReal;
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code !== "ENOENT") throw error;
    return path.join(realParent, path.basename(candidate));
  }
}

function relativeDisplay(absolutePath: string): string {
  const rel = path.relative(root, absolutePath);
  return rel === "" ? "." : rel;
}

async function listTree(startRelative: string, maxDepth: number): Promise<string[]> {
  const start = await safeExistingPath(startRelative);
  const stat = await fs.stat(start);
  if (!stat.isDirectory()) throw new Error("Path is not a directory.");

  const output: string[] = [];

  async function walk(dir: string, depth: number): Promise<void> {
    if (output.length >= MAX_LIST_ENTRIES) return;
    const entries = await fs.readdir(dir, { withFileTypes: true });
    entries.sort((a, b) => a.name.localeCompare(b.name));

    for (const entry of entries) {
      if (output.length >= MAX_LIST_ENTRIES) break;
      const absolute = path.join(dir, entry.name);
      const rel = relativeDisplay(absolute);

      if (entry.isSymbolicLink()) {
        output.push(`${rel} -> [symlink]`);
        continue;
      }

      if (entry.isDirectory()) {
        output.push(`${rel}/`);
        if (depth < maxDepth) await walk(absolute, depth + 1);
      } else {
        output.push(rel);
      }
    }
  }

  await walk(start, 0);
  return output;
}

function textResult(value: unknown) {
  return { content: [{ type: "text" as const, text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }] };
}

function createServer(): McpServer {
  const server = new McpServer({
    name: "codex-mcp",
    version: "0.1.0",
  });

  server.registerTool(
    "project_info",
    {
      title: "Project info",
      description: "Show the allowed PROJECT_ROOT and filesystem limits.",
      inputSchema: {},
    },
    async () => textResult({ projectRoot: root, maxReadBytes: MAX_READ_BYTES, maxListEntries: MAX_LIST_ENTRIES }),
  );

  server.registerTool(
    "list_files",
    {
      title: "List project files",
      description: "List files and directories inside PROJECT_ROOT. Paths must be relative to PROJECT_ROOT.",
      inputSchema: {
        path: z.string().default("."),
        maxDepth: z.number().int().min(0).max(10).default(4),
      },
    },
    async ({ path: requestedPath, maxDepth }) => {
      const files = await listTree(requestedPath, maxDepth);
      return textResult(files.join("\n") || "(empty directory)");
    },
  );

  server.registerTool(
    "read_file",
    {
      title: "Read file",
      description: "Read one UTF-8 text file inside PROJECT_ROOT.",
      inputSchema: {
        path: z.string().min(1),
      },
    },
    async ({ path: requestedPath }) => {
      const file = await safeExistingPath(requestedPath);
      const stat = await fs.stat(file);
      if (!stat.isFile()) throw new Error("Path is not a file.");
      if (stat.size > MAX_READ_BYTES) throw new Error(`File exceeds ${MAX_READ_BYTES} byte read limit.`);
      const content = await fs.readFile(file, "utf8");
      return textResult(content);
    },
  );

  server.registerTool(
    "write_file",
    {
      title: "Create or overwrite file",
      description: "Create or fully overwrite a UTF-8 text file inside PROJECT_ROOT. Parent directory must already exist.",
      inputSchema: {
        path: z.string().min(1),
        content: z.string(),
      },
    },
    async ({ path: requestedPath, content }) => {
      const file = await safeWritePath(requestedPath);
      await fs.writeFile(file, content, "utf8");
      return textResult(`Wrote ${Buffer.byteLength(content, "utf8")} bytes to ${relativeDisplay(file)}`);
    },
  );

  server.registerTool(
    "edit_file",
    {
      title: "Edit file by exact text replacement",
      description: "Replace exact text in a UTF-8 file. By default the old text must occur exactly once, which prevents ambiguous edits.",
      inputSchema: {
        path: z.string().min(1),
        oldText: z.string().min(1),
        newText: z.string(),
        replaceAll: z.boolean().default(false),
      },
    },
    async ({ path: requestedPath, oldText, newText, replaceAll }) => {
      const file = await safeExistingPath(requestedPath);
      const stat = await fs.stat(file);
      if (!stat.isFile()) throw new Error("Path is not a file.");
      if (stat.size > MAX_READ_BYTES) throw new Error(`File exceeds ${MAX_READ_BYTES} byte edit limit.`);

      const original = await fs.readFile(file, "utf8");
      const occurrences = original.split(oldText).length - 1;
      if (occurrences === 0) throw new Error("oldText was not found.");
      if (!replaceAll && occurrences !== 1) {
        throw new Error(`oldText occurs ${occurrences} times. Make oldText more specific or set replaceAll=true.`);
      }

      const updated = replaceAll ? original.split(oldText).join(newText) : original.replace(oldText, newText);
      await fs.writeFile(file, updated, "utf8");
      return textResult(`Edited ${relativeDisplay(file)} (${replaceAll ? occurrences : 1} replacement${replaceAll && occurrences !== 1 ? "s" : ""}).`);
    },
  );

  return server;
}

const app = express();
app.use(express.json({ limit: "2mb" }));

app.get("/", (_req, res) => {
  res.json({ name: "codex-mcp", status: "ok", endpoint: "/mcp", projectRoot: root });
});

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
        onsessioninitialized: (id) => {
          transports[id] = transport;
        },
      });

      transport.onclose = () => {
        if (transport.sessionId) delete transports[transport.sessionId];
      };

      const server = createServer();
      await server.connect(transport);
    } else {
      res.status(400).json({
        jsonrpc: "2.0",
        error: { code: -32000, message: "Invalid or missing MCP session." },
        id: null,
      });
      return;
    }

    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    console.error(error);
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
    res.status(400).send("Invalid or missing MCP session ID.");
    return;
  }
  await transports[sessionId].handleRequest(req, res);
}

app.get("/mcp", handleSessionRequest);
app.delete("/mcp", handleSessionRequest);

app.listen(PORT, HOST, () => {
  console.log(`codex-mcp listening on http://${HOST}:${PORT}/mcp`);
  console.log(`PROJECT_ROOT=${root}`);
});
