import { promises as fs } from "node:fs";
import path from "node:path";
import WebSocket from "ws";

const SERVER_URL = process.env.SERVER_URL;
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const PROJECT_ROOT = process.env.PROJECT_ROOT;
const MAX_READ_BYTES = 1024 * 1024;
const MAX_LIST_ENTRIES = 5000;

if (!SERVER_URL || !DEVICE_TOKEN || !PROJECT_ROOT) {
  console.error("Required: SERVER_URL, DEVICE_TOKEN, PROJECT_ROOT");
  process.exit(1);
}

const root = await fs.realpath(path.resolve(PROJECT_ROOT));
const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
function isInsideRoot(candidate: string) { return candidate === root || candidate.startsWith(rootPrefix); }
function lexicalPath(relativePath: string) {
  if (path.isAbsolute(relativePath)) throw new Error("Absolute paths are not allowed.");
  const candidate = path.resolve(root, relativePath || ".");
  if (!isInsideRoot(candidate)) throw new Error("Path escapes PROJECT_ROOT.");
  return candidate;
}
async function safeExistingPath(relativePath: string) {
  const real = await fs.realpath(lexicalPath(relativePath));
  if (!isInsideRoot(real)) throw new Error("Resolved path escapes PROJECT_ROOT.");
  return real;
}
async function safeWritePath(relativePath: string) {
  const candidate = lexicalPath(relativePath);
  const realParent = await fs.realpath(path.dirname(candidate));
  if (!isInsideRoot(realParent)) throw new Error("Parent directory escapes PROJECT_ROOT.");
  try {
    const real = await fs.realpath(candidate);
    if (!isInsideRoot(real)) throw new Error("Existing file escapes PROJECT_ROOT.");
    return real;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    return path.join(realParent, path.basename(candidate));
  }
}
function rel(p: string) { const v = path.relative(root, p); return v || "."; }

async function listFiles(startRelative = ".", maxDepth = 4) {
  const start = await safeExistingPath(startRelative);
  if (!(await fs.stat(start)).isDirectory()) throw new Error("Path is not a directory.");
  const out: string[] = [];
  async function walk(dir: string, depth: number) {
    if (out.length >= MAX_LIST_ENTRIES) return;
    const entries = (await fs.readdir(dir, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      if (out.length >= MAX_LIST_ENTRIES) break;
      const absolute = path.join(dir, entry.name);
      const display = rel(absolute);
      if (entry.isSymbolicLink()) { out.push(`${display} -> [symlink]`); continue; }
      if (entry.isDirectory()) { out.push(`${display}/`); if (depth < maxDepth) await walk(absolute, depth + 1); }
      else out.push(display);
    }
  }
  await walk(start, 0);
  return out;
}

async function handleTool(tool: string, args: any) {
  if (tool === "project_info") return { projectRoot: root, maxReadBytes: MAX_READ_BYTES, maxListEntries: MAX_LIST_ENTRIES };
  if (tool === "list_files") return { files: await listFiles(args.path ?? ".", args.maxDepth ?? 4) };
  if (tool === "read_file") {
    const file = await safeExistingPath(args.path);
    const stat = await fs.stat(file);
    if (!stat.isFile()) throw new Error("Path is not a file.");
    if (stat.size > MAX_READ_BYTES) throw new Error("File is too large.");
    return { path: rel(file), content: await fs.readFile(file, "utf8") };
  }
  if (tool === "write_file") {
    const file = await safeWritePath(args.path);
    await fs.writeFile(file, String(args.content ?? ""), "utf8");
    return { path: rel(file), bytes: Buffer.byteLength(String(args.content ?? ""), "utf8") };
  }
  if (tool === "edit_file") {
    const file = await safeExistingPath(args.path);
    const stat = await fs.stat(file);
    if (!stat.isFile()) throw new Error("Path is not a file.");
    if (stat.size > MAX_READ_BYTES) throw new Error("File is too large.");
    const original = await fs.readFile(file, "utf8");
    const oldText = String(args.oldText);
    const newText = String(args.newText ?? "");
    const count = original.split(oldText).length - 1;
    if (count === 0) throw new Error("oldText was not found.");
    if (!args.replaceAll && count !== 1) throw new Error(`oldText occurs ${count} times.`);
    const updated = args.replaceAll ? original.split(oldText).join(newText) : original.replace(oldText, newText);
    await fs.writeFile(file, updated, "utf8");
    return { path: rel(file), replacements: args.replaceAll ? count : 1 };
  }
  throw new Error(`Unknown tool: ${tool}`);
}

function connect() {
  const ws = new WebSocket(SERVER_URL);
  ws.on("open", () => ws.send(JSON.stringify({ type: "register", token: DEVICE_TOKEN })));
  ws.on("message", async (raw) => {
    const msg = JSON.parse(raw.toString());
    if (msg.type === "registered") { console.log(`Connected. PROJECT_ROOT=${root}`); return; }
    if (msg.type !== "tool_call") return;
    try {
      const result = await handleTool(msg.tool, msg.args ?? {});
      ws.send(JSON.stringify({ type: "tool_result", id: msg.id, ok: true, result }));
    } catch (error) {
      ws.send(JSON.stringify({ type: "tool_result", id: msg.id, ok: false, error: error instanceof Error ? error.message : String(error) }));
    }
  });
  ws.on("close", () => { console.log("Disconnected; reconnecting..."); setTimeout(connect, 2000); });
  ws.on("error", (error) => console.error("WebSocket error:", error.message));
}
connect();
