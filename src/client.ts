import { promises as fs } from "node:fs";
import path from "node:path";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { randomUUID } from "node:crypto";
import WebSocket from "ws";

const SERVER_URL = process.env.SERVER_URL;
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const PROJECT_ROOT = process.env.PROJECT_ROOT;
const ALLOW_SHELL = process.env.CODELOCAL_ALLOW_SHELL === "1";
const MAX_READ_BYTES = 1024 * 1024;
const MAX_BATCH_BYTES = 4 * 1024 * 1024;
const MAX_LIST_ENTRIES = 5000;
const MAX_OUTPUT_BYTES = 2 * 1024 * 1024;

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
  if (!isInsideRoot(real)) throw new Error("Resolved path escapes PROJECT_ROOT (possible symlink traversal).");
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
      if (entry.name === ".git" || entry.name === "node_modules") { out.push(`${display}/ [skipped]`); continue; }
      if (entry.isSymbolicLink()) { out.push(`${display} -> [symlink]`); continue; }
      if (entry.isDirectory()) { out.push(`${display}/`); if (depth < maxDepth) await walk(absolute, depth + 1); }
      else out.push(display);
    }
  }
  await walk(start, 0);
  return out;
}

function runProcess(command: string, args: string[], opts: { cwd?: string; input?: string; timeoutMs?: number } = {}) {
  return new Promise<{ stdout: string; stderr: string; exitCode: number | null }>((resolve, reject) => {
    const child = spawn(command, args, { cwd: opts.cwd ?? root, env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat" }, stdio: "pipe" });
    let stdout = ""; let stderr = ""; let done = false;
    const append = (current: string, chunk: Buffer) => (current + chunk.toString()).slice(-MAX_OUTPUT_BYTES);
    child.stdout.on("data", (d) => { stdout = append(stdout, d); });
    child.stderr.on("data", (d) => { stderr = append(stderr, d); });
    child.on("error", reject);
    const timer = opts.timeoutMs ? setTimeout(() => { if (!done) child.kill("SIGTERM"); }, opts.timeoutMs) : null;
    child.on("close", (code) => { done = true; if (timer) clearTimeout(timer); resolve({ stdout, stderr, exitCode: code }); });
    if (opts.input !== undefined) { child.stdin.write(opts.input); child.stdin.end(); }
  });
}

async function readOne(requestedPath: string) {
  const file = await safeExistingPath(requestedPath);
  const stat = await fs.stat(file);
  if (!stat.isFile()) throw new Error(`${requestedPath}: not a file.`);
  if (stat.size > MAX_READ_BYTES) throw new Error(`${requestedPath}: exceeds ${MAX_READ_BYTES} byte read limit.`);
  return { path: rel(file), content: await fs.readFile(file, "utf8"), bytes: stat.size };
}

async function searchCode(query: string, requestedPath = ".", maxResults = 200, fixedStrings = false) {
  const cwd = await safeExistingPath(requestedPath);
  const rgArgs = ["--line-number", "--column", "--no-heading", "--color", "never", "--hidden", "--glob", "!.git/**", "--glob", "!node_modules/**"];
  if (fixedStrings) rgArgs.push("--fixed-strings");
  rgArgs.push("--", query, ".");
  try {
    const result = await runProcess("rg", rgArgs, { cwd, timeoutMs: 20_000 });
    const lines = result.stdout.split("\n").filter(Boolean).slice(0, maxResults);
    return { engine: "ripgrep", matches: lines, truncated: result.stdout.split("\n").filter(Boolean).length > maxResults };
  } catch {
    const grepArgs = ["-RIn", "--exclude-dir=.git", "--exclude-dir=node_modules", "--", query, "."];
    const result = await runProcess("grep", grepArgs, { cwd, timeoutMs: 20_000 });
    const lines = result.stdout.split("\n").filter(Boolean).slice(0, maxResults);
    return { engine: "grep", matches: lines, truncated: result.stdout.split("\n").filter(Boolean).length > maxResults };
  }
}

function validatePatchPaths(patchText: string) {
  for (const line of patchText.split("\n")) {
    if (!line.startsWith("+++ ") && !line.startsWith("--- ")) continue;
    const raw = line.slice(4).trim().split("\t")[0];
    if (raw === "/dev/null") continue;
    const normalized = raw.replace(/^[ab]\//, "");
    if (path.isAbsolute(normalized) || normalized.split(/[\\/]+/).includes("..")) throw new Error(`Unsafe patch path: ${raw}`);
    lexicalPath(normalized);
  }
}

const processes = new Map<string, { child: ChildProcessWithoutNullStreams; output: string; exitCode: number | null; startedAt: number }>();
function appendProcessOutput(id: string, chunk: Buffer, prefix = "") {
  const proc = processes.get(id); if (!proc) return;
  proc.output = (proc.output + prefix + chunk.toString()).slice(-MAX_OUTPUT_BYTES);
}

async function startShell(command: string, cwdRelative = ".", yieldMs = 1000, timeoutMs = 0) {
  if (!ALLOW_SHELL) throw new Error("Shell execution is disabled. Restart client with CODELOCAL_ALLOW_SHELL=1 to enable it.");
  const cwd = await safeExistingPath(cwdRelative);
  if (!(await fs.stat(cwd)).isDirectory()) throw new Error("cwd is not a directory.");
  const id = randomUUID();
  const child = spawn(command, { cwd, env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat" }, shell: true, stdio: "pipe" }) as ChildProcessWithoutNullStreams;
  processes.set(id, { child, output: "", exitCode: null, startedAt: Date.now() });
  child.stdout.on("data", (d) => appendProcessOutput(id, d));
  child.stderr.on("data", (d) => appendProcessOutput(id, d, "[stderr] "));
  child.on("close", (code) => { const p = processes.get(id); if (p) p.exitCode = code; });
  child.on("error", (e) => { const p = processes.get(id); if (p) { p.output += `\n[process error] ${e.message}`; p.exitCode = -1; } });
  if (timeoutMs > 0) setTimeout(() => { const p = processes.get(id); if (p && p.exitCode === null) p.child.kill("SIGTERM"); }, timeoutMs);
  await new Promise((r) => setTimeout(r, Math.min(Math.max(yieldMs, 0), 10_000)));
  const p = processes.get(id)!;
  return { processId: id, running: p.exitCode === null, exitCode: p.exitCode, output: p.output };
}

async function handleTool(tool: string, args: any) {
  if (tool === "project_info") {
    const packageJson = await fs.readFile(path.join(root, "package.json"), "utf8").then(JSON.parse).catch(() => null);
    const git = await runProcess("git", ["rev-parse", "--show-toplevel"], { cwd: root, timeoutMs: 5000 }).catch(() => null);
    return { projectRoot: root, projectName: packageJson?.name ?? path.basename(root), gitRepository: !!git && git.exitCode === 0, shellEnabled: ALLOW_SHELL, capabilities: ["filesystem", "batch-read", "search", "patch", "git", "shell", "processes"] };
  }
  if (tool === "list_files") return { files: await listFiles(args.path ?? ".", args.maxDepth ?? 4) };
  if (tool === "read_file") return readOne(args.path);
  if (tool === "read_files") {
    const paths = Array.isArray(args.paths) ? args.paths : [];
    if (paths.length > 50) throw new Error("read_files supports at most 50 files per call.");
    const files = []; let total = 0;
    for (const p of paths) { const file = await readOne(String(p)); total += file.bytes; if (total > MAX_BATCH_BYTES) throw new Error("Batch exceeds 4 MiB limit."); files.push(file); }
    return { files, totalBytes: total };
  }
  if (tool === "search_code") return searchCode(String(args.query), args.path ?? ".", args.maxResults ?? 200, !!args.fixedStrings);
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
    const oldText = String(args.oldText); const newText = String(args.newText ?? "");
    const count = original.split(oldText).length - 1;
    if (count === 0) throw new Error("oldText was not found.");
    if (!args.replaceAll && count !== 1) throw new Error(`oldText occurs ${count} times.`);
    const updated = args.replaceAll ? original.split(oldText).join(newText) : original.replace(oldText, newText);
    await fs.writeFile(file, updated, "utf8");
    return { path: rel(file), replacements: args.replaceAll ? count : 1 };
  }
  if (tool === "apply_patch") {
    const patchText = String(args.patch ?? "");
    if (!patchText.trim()) throw new Error("Patch is empty.");
    validatePatchPaths(patchText);
    const check = await runProcess("git", ["apply", "--check", "--whitespace=nowarn", "-"], { cwd: root, input: patchText, timeoutMs: 20_000 });
    if (check.exitCode !== 0) throw new Error(`Patch check failed:\n${check.stderr || check.stdout}`);
    const applied = await runProcess("git", ["apply", "--whitespace=nowarn", "-"], { cwd: root, input: patchText, timeoutMs: 20_000 });
    if (applied.exitCode !== 0) throw new Error(`Patch failed:\n${applied.stderr || applied.stdout}`);
    return { applied: true };
  }
  if (tool === "git_status") {
    const r = await runProcess("git", ["status", "--short", "--branch"], { cwd: root, timeoutMs: 10_000 });
    return { exitCode: r.exitCode, output: r.stdout + r.stderr };
  }
  if (tool === "git_diff") {
    const argv = ["diff", "--no-ext-diff", "--unified=3"]; if (args.cached) argv.push("--cached"); if (args.path) argv.push("--", String(args.path));
    const r = await runProcess("git", argv, { cwd: root, timeoutMs: 20_000 });
    return { exitCode: r.exitCode, diff: (r.stdout + r.stderr).slice(-MAX_OUTPUT_BYTES) };
  }
  if (tool === "run_command") return startShell(String(args.command), args.cwd ?? ".", args.yieldMs ?? 1000, args.timeoutMs ?? 0);
  if (tool === "process_poll") {
    const p = processes.get(String(args.processId)); if (!p) throw new Error("Unknown processId.");
    return { processId: args.processId, running: p.exitCode === null, exitCode: p.exitCode, output: p.output, elapsedMs: Date.now() - p.startedAt };
  }
  if (tool === "process_write") {
    const p = processes.get(String(args.processId)); if (!p) throw new Error("Unknown processId."); if (p.exitCode !== null) throw new Error("Process already exited.");
    p.child.stdin.write(String(args.input ?? "")); return { written: true };
  }
  if (tool === "process_kill") {
    const p = processes.get(String(args.processId)); if (!p) throw new Error("Unknown processId."); if (p.exitCode === null) p.child.kill(args.signal === "SIGKILL" ? "SIGKILL" : "SIGTERM");
    return { killed: true };
  }
  throw new Error(`Unknown tool: ${tool}`);
}

function connect() {
  const ws = new WebSocket(SERVER_URL);
  ws.on("open", () => ws.send(JSON.stringify({ type: "register", token: DEVICE_TOKEN })));
  ws.on("message", async (raw) => {
    const msg = JSON.parse(raw.toString());
    if (msg.type === "registered") { console.log(`Connected. PROJECT_ROOT=${root}`); console.log(`Shell: ${ALLOW_SHELL ? "ENABLED" : "disabled (set CODELOCAL_ALLOW_SHELL=1)"}`); return; }
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
