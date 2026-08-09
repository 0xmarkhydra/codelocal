import { promises as fs } from "node:fs";
import path from "node:path";
import os from "node:os";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { createInterface } from "node:readline/promises";
import WebSocket from "ws";
import ignore, { type Ignore } from "ignore";
import chokidar from "chokidar";
import { log, summarizeToolArgs } from "./log.js";
import { TypeScriptSemanticIndex } from "./semantic.js";
import { VERSION } from "./version.js";

const SERVER_URL = process.env.SERVER_URL;
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const PROJECT_ROOT = process.env.PROJECT_ROOT;
const DEVICE_ID = process.env.CODELOCAL_DEVICE_ID ?? os.hostname();
const WORKSPACE_ID = process.env.CODELOCAL_WORKSPACE_ID ?? path.basename(PROJECT_ROOT || "workspace");
const WORKSPACE_NAME = process.env.CODELOCAL_WORKSPACE_NAME ?? path.basename(PROJECT_ROOT || "workspace");
const ALLOW_SHELL = process.env.CODELOCAL_ALLOW_SHELL === "1";
const ALLOW_DANGEROUS = process.env.CODELOCAL_ALLOW_DANGEROUS === "1";
const APPROVAL_MODE = (process.env.CODELOCAL_APPROVAL_MODE ?? "prompt").toLowerCase();
const MIRROR_PROCESS_OUTPUT = process.env.CODELOCAL_MIRROR_PROCESS_OUTPUT !== "0";
const MAX_READ_BYTES = 2 * 1024 * 1024;
const MAX_BATCH_BYTES = 8 * 1024 * 1024;
const MAX_LIST_ENTRIES = 10000;
const MAX_OUTPUT_BYTES = 2 * 1024 * 1024;
const MAX_PROCESSES = 32;

if (!SERVER_URL || !DEVICE_TOKEN || !PROJECT_ROOT) {
  console.error("Required: SERVER_URL, DEVICE_TOKEN, PROJECT_ROOT");
  process.exit(1);
}

const root = await fs.realpath(path.resolve(PROJECT_ROOT));
const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
const semantic = new TypeScriptSemanticIndex(root);
let ignoreMatcher: Ignore = ignore();
let gitignoreLoaded = false;
let metadataEpoch = 0;

function isInsideRoot(candidate: string) { return candidate === root || candidate.startsWith(rootPrefix); }
function rel(p: string) { const value = path.relative(root, p); return value.split(path.sep).join("/") || "."; }
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

function isSensitivePath(relativePath: string) {
  const p = relativePath.replace(/\\/g, "/").replace(/^\.\//, "");
  const base = path.posix.basename(p).toLowerCase();
  if ([".env.example", ".env.sample", ".env.template"].includes(base)) return false;
  if (/(^|\/)\.(ssh|aws|gnupg|gcloud)(\/|$)/i.test(p)) return true;
  if (/(^|\/)\.env($|\.)/i.test(p)) return true;
  if (/\.(pem|p12|pfx|key)$/i.test(base)) return true;
  if (/(credentials?|service[-_]?account|private[-_]?key|secrets?)\.(json|ya?ml|toml|ini)$/i.test(base)) return true;
  return false;
}
function assertNotSensitive(relativePath: string) {
  if (isSensitivePath(relativePath)) throw new Error(`Access blocked by sensitive-path policy: ${relativePath}`);
}

async function reloadIgnore() {
  const matcher = ignore();
  matcher.add([".git/", ".DS_Store"]);
  try { matcher.add(await fs.readFile(path.join(root, ".gitignore"), "utf8")); } catch {}
  try { matcher.add(await fs.readFile(path.join(root, ".git", "info", "exclude"), "utf8")); } catch {}
  ignoreMatcher = matcher;
  gitignoreLoaded = true;
  metadataEpoch++;
}
await reloadIgnore();
function isIgnored(relativePath: string) {
  const p = relativePath.replace(/\\/g, "/").replace(/^\.\//, "");
  if (!p || p === ".") return false;
  return ignoreMatcher.ignores(p) || ignoreMatcher.ignores(p.endsWith("/") ? p : `${p}/`);
}

function hashBuffer(buf: Buffer) { return createHash("sha256").update(buf).digest("hex"); }
async function fileMeta(file: string) {
  const stat = await fs.stat(file);
  const relative = rel(file);
  assertNotSensitive(relative);
  let hash: string | null = null;
  if (stat.isFile() && stat.size <= MAX_READ_BYTES * 4) hash = hashBuffer(await fs.readFile(file));
  return { path: relative, size: stat.size, mtimeMs: stat.mtimeMs, hash, ignored: isIgnored(relative), isFile: stat.isFile(), isDirectory: stat.isDirectory() };
}
async function isBinary(file: string) {
  const handle = await fs.open(file, "r");
  try {
    const buf = Buffer.alloc(8192);
    const { bytesRead } = await handle.read(buf, 0, buf.length, 0);
    for (let i = 0; i < bytesRead; i++) if (buf[i] === 0) return true;
    return false;
  } finally { await handle.close(); }
}

async function readOne(requestedPath: string, startLine?: number, endLine?: number) {
  assertNotSensitive(requestedPath);
  const file = await safeExistingPath(requestedPath);
  const stat = await fs.stat(file);
  if (!stat.isFile()) throw new Error(`${requestedPath}: not a file.`);
  if (stat.size > MAX_READ_BYTES) throw new Error(`${requestedPath}: exceeds ${MAX_READ_BYTES} byte read limit; use read_file_range for text files or inspect metadata.`);
  if (await isBinary(file)) return { ...(await fileMeta(file)), binary: true, content: null };
  const buf = await fs.readFile(file);
  const text = buf.toString("utf8");
  const lines = text.split(/\r?\n/);
  const from = Math.max(1, startLine ?? 1);
  const to = Math.min(lines.length, endLine ?? lines.length);
  const content = lines.slice(from - 1, to).join("\n");
  return { ...(await fileMeta(file)), binary: false, startLine: from, endLine: to, totalLines: lines.length, content };
}

async function listFiles(startRelative = ".", maxDepth = 4, includeIgnored = false) {
  const start = await safeExistingPath(startRelative);
  if (!(await fs.stat(start)).isDirectory()) throw new Error("Path is not a directory.");
  const out: Array<{ path: string; type: "file" | "directory" | "symlink"; ignored?: boolean; sensitive?: boolean }> = [];
  async function walk(dir: string, depth: number) {
    if (out.length >= MAX_LIST_ENTRIES) return;
    const entries = (await fs.readdir(dir, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      if (out.length >= MAX_LIST_ENTRIES) break;
      const absolute = path.join(dir, entry.name);
      const display = rel(absolute);
      const sensitive = isSensitivePath(display);
      const ignored = isIgnored(display);
      if (sensitive) { out.push({ path: display, type: entry.isDirectory() ? "directory" : "file", sensitive: true }); continue; }
      if (ignored && !includeIgnored) continue;
      if (entry.isSymbolicLink()) { out.push({ path: display, type: "symlink", ignored }); continue; }
      if (entry.isDirectory()) { out.push({ path: `${display}/`, type: "directory", ignored }); if (depth < maxDepth) await walk(absolute, depth + 1); }
      else out.push({ path: display, type: "file", ignored });
    }
  }
  await walk(start, 0);
  return { entries: out, truncated: out.length >= MAX_LIST_ENTRIES, includeIgnored };
}

function runProcess(command: string, args: string[], opts: { cwd?: string; input?: string; timeoutMs?: number } = {}) {
  return new Promise<{ stdout: string; stderr: string; exitCode: number | null }>((resolve, reject) => {
    const child = spawn(command, args, { cwd: opts.cwd ?? root, env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CI: process.env.CI ?? "1" }, stdio: "pipe" });
    let stdout = "", stderr = "", done = false;
    const append = (current: string, chunk: Buffer) => (current + chunk.toString()).slice(-MAX_OUTPUT_BYTES);
    child.stdout.on("data", (d) => { stdout = append(stdout, d); });
    child.stderr.on("data", (d) => { stderr = append(stderr, d); });
    child.on("error", reject);
    const timer = opts.timeoutMs ? setTimeout(() => { if (!done) child.kill("SIGTERM"); }, opts.timeoutMs) : null;
    child.on("close", (code) => { done = true; if (timer) clearTimeout(timer); resolve({ stdout, stderr, exitCode: code }); });
    if (opts.input !== undefined) { child.stdin.write(opts.input); child.stdin.end(); }
  });
}

async function searchCode(query: string, requestedPath = ".", maxResults = 200, fixedStrings = false, includeIgnored = false) {
  const cwd = await safeExistingPath(requestedPath);
  const rgArgs = ["--line-number", "--column", "--no-heading", "--color", "never", "--hidden"];
  if (includeIgnored) rgArgs.push("--no-ignore");
  if (fixedStrings) rgArgs.push("--fixed-strings");
  rgArgs.push("--", query, ".");
  const result = await runProcess("rg", rgArgs, { cwd, timeoutMs: 30_000 }).catch(async () => runProcess("grep", ["-RIn", "--", query, "."], { cwd, timeoutMs: 30_000 }));
  const all = result.stdout.split("\n").filter(Boolean).filter((line) => !isSensitivePath(line.split(":", 1)[0]));
  return { matches: all.slice(0, maxResults), truncated: all.length > maxResults, includeIgnored };
}

function validatePatchPaths(patchText: string) {
  for (const line of patchText.split("\n")) {
    if (!line.startsWith("+++ ") && !line.startsWith("--- ")) continue;
    const raw = line.slice(4).trim().split("\t")[0];
    if (raw === "/dev/null") continue;
    const normalized = raw.replace(/^[ab]\//, "");
    if (path.isAbsolute(normalized) || normalized.split(/[\\/]+/).includes("..")) throw new Error(`Unsafe patch path: ${raw}`);
    assertNotSensitive(normalized);
    lexicalPath(normalized);
  }
}

async function requestApproval(operation: string, detail: string) {
  if (APPROVAL_MODE === "auto" && ALLOW_DANGEROUS) return true;
  if (APPROVAL_MODE === "deny") return false;
  if (!process.stdin.isTTY || !process.stdout.isTTY) return false;
  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    const answer = await rl.question(`\n[CodeLocal approval required]\n${operation}\n${detail}\nAllow? [y/N] `);
    return /^y(es)?$/i.test(answer.trim());
  } finally { rl.close(); }
}

function classifyCommand(command: string) {
  const normalized = command.replace(/\s+/g, " ").trim();
  const blocked: Array<[RegExp, string]> = [
    [/(^|[;&|]\s*)sudo\b/i, "sudo/system privilege escalation"],
    [/(^|[;&|]\s*)(shutdown|reboot|halt|diskutil|mkfs|fdisk|gpt|mount|umount)\b/i, "system/disk administration"],
    [/\bdd\s+[^\n]*\bof=\/dev\//i, "raw device write"],
    [/(^|\s)\.\.\/(?:\.\.\/)?/, "parent-directory traversal"],
    [/(^|\s)(~\/)?\.(ssh|aws|gnupg)(\/|\s|$)/i, "credential directory access"],
  ];
  const approvals: Array<[RegExp, string]> = [
    [/\b(npm|pnpm|yarn|bun)\s+(install|add|remove|uninstall|update|upgrade)\b/i, "package dependency change"],
    [/\b(git\s+(commit|push|reset\s+--hard|clean\s+-[a-z]*f))\b/i, "Git write/destructive action"],
    [/\b(prisma|typeorm|sequelize|knex|alembic|rails)\b[^\n]*(migrate|migration|db:)/i, "database migration"],
    [/\b(curl|wget|ssh|scp|sftp)\b/i, "network/remote command"],
    [/\brm\s+-[^\s]*r/i, "recursive delete"],
  ];
  const blockReasons = blocked.filter(([r]) => r.test(normalized)).map(([, reason]) => reason);
  const approvalReasons = approvals.filter(([r]) => r.test(normalized)).map(([, reason]) => reason);
  return { normalized, blocked: blockReasons, approvalRequired: approvalReasons.length > 0, approvalReasons };
}

type ProcessRecord = { child: ChildProcessWithoutNullStreams; output: string; baseOffset: number; totalBytes: number; exitCode: number | null; startedAt: number; command: string; cwd: string };
const processes = new Map<string, ProcessRecord>();
function appendProcessOutput(id: string, chunk: Buffer, prefix = "") {
  const proc = processes.get(id); if (!proc) return;
  const text = prefix + chunk.toString();
  proc.totalBytes += Buffer.byteLength(text, "utf8"); proc.output += text;
  if (MIRROR_PROCESS_OUTPUT) process.stdout.write(`[proc:${id.slice(0, 8)}] ${text}`);
  if (Buffer.byteLength(proc.output, "utf8") > MAX_OUTPUT_BYTES) {
    const before = Buffer.byteLength(proc.output, "utf8"); proc.output = proc.output.slice(-MAX_OUTPUT_BYTES); proc.baseOffset += before - Buffer.byteLength(proc.output, "utf8");
  }
}
function processSnapshot(id: string, cursor?: number) {
  const proc = processes.get(id); if (!proc) throw new Error("Unknown processId.");
  const requested = Math.max(cursor ?? proc.baseOffset, proc.baseOffset);
  const relativeOffset = Math.max(0, requested - proc.baseOffset);
  return { processId: id, running: proc.exitCode === null, exitCode: proc.exitCode, output: proc.output.slice(relativeOffset), cursor: proc.totalBytes, truncatedBeforeCursor: (cursor ?? proc.baseOffset) < proc.baseOffset, elapsedMs: Date.now() - proc.startedAt, command: proc.command, cwd: rel(proc.cwd) };
}
function pruneProcesses() {
  const finished = [...processes.entries()].filter(([, p]) => p.exitCode !== null).sort((a, b) => a[1].startedAt - b[1].startedAt);
  while (processes.size >= MAX_PROCESSES && finished.length) processes.delete(finished.shift()![0]);
  if (processes.size >= MAX_PROCESSES) throw new Error(`Too many active processes (${MAX_PROCESSES}).`);
}
async function startShell(command: string, cwdRelative = ".", yieldMs = 1000, timeoutMs = 0) {
  if (!ALLOW_SHELL) throw new Error("Shell execution is disabled. Restart client with CODELOCAL_ALLOW_SHELL=1.");
  const policy = classifyCommand(command);
  if (policy.blocked.length && !ALLOW_DANGEROUS) throw new Error(`Command blocked: ${policy.blocked.join("; ")}`);
  if (policy.approvalRequired && !(await requestApproval("Run command", `${policy.normalized}\nReason: ${policy.approvalReasons.join("; ")}`))) throw new Error("Command denied by local approval policy.");
  const cwd = await safeExistingPath(cwdRelative);
  pruneProcesses();
  const id = randomUUID();
  const child = spawn(command, { cwd, env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CI: process.env.CI ?? "1" }, shell: true, stdio: "pipe" }) as ChildProcessWithoutNullStreams;
  processes.set(id, { child, output: "", baseOffset: 0, totalBytes: 0, exitCode: null, startedAt: Date.now(), command, cwd });
  log("info", "process.started", { processId: id, command: policy.normalized.slice(0, 500), cwd: rel(cwd) });
  child.stdout.on("data", (d) => appendProcessOutput(id, d)); child.stderr.on("data", (d) => appendProcessOutput(id, d, "[stderr] "));
  child.on("close", (code) => { const p = processes.get(id); if (p) p.exitCode = code; log("info", "process.exited", { processId: id, exitCode: code }); });
  child.on("error", (e) => { appendProcessOutput(id, Buffer.from(`\n[process error] ${e.message}\n`)); const p = processes.get(id); if (p) p.exitCode = -1; });
  if (timeoutMs > 0) setTimeout(() => { const p = processes.get(id); if (p && p.exitCode === null) { appendProcessOutput(id, Buffer.from(`\n[CodeLocal] timeout after ${timeoutMs}ms\n`)); p.child.kill("SIGTERM"); } }, timeoutMs);
  await new Promise((r) => setTimeout(r, Math.min(Math.max(yieldMs, 0), 10_000)));
  return processSnapshot(id, 0);
}

async function readInstructions(requestedPath = ".") {
  const target = await safeExistingPath(requestedPath); const stat = await fs.stat(target); const targetDir = stat.isDirectory() ? target : path.dirname(target);
  const dirs: string[] = []; let current = targetDir;
  while (isInsideRoot(current)) { dirs.push(current); if (current === root) break; current = path.dirname(current); }
  dirs.reverse(); const files: Array<{ path: string; content: string }> = [];
  for (const dir of dirs) { const candidate = rel(dir) === "." ? "AGENTS.md" : `${rel(dir)}/AGENTS.md`; try { const r = await readOne(candidate); if (!r.binary && r.content) files.push({ path: candidate, content: r.content }); } catch {} }
  for (const candidate of ["CLAUDE.md", ".github/copilot-instructions.md"]) { try { const r = await readOne(candidate); if (!r.binary && r.content) files.push({ path: candidate, content: r.content }); } catch {} }
  return { requestedPath, instructionFiles: files };
}

async function inspectDependency(name: string) {
  const pkgPath = lexicalPath(`node_modules/${name}/package.json`); assertNotSensitive(rel(pkgPath));
  try {
    const pkg = JSON.parse(await fs.readFile(pkgPath, "utf8"));
    return { ecosystem: "node", name: pkg.name ?? name, version: pkg.version ?? null, type: pkg.type ?? null, main: pkg.main ?? null, module: pkg.module ?? null, types: pkg.types ?? pkg.typings ?? null, exports: pkg.exports ?? null, dependencies: pkg.dependencies ?? null, packagePath: rel(pkgPath) };
  } catch {
    const packageJson = JSON.parse(await fs.readFile(path.join(root, "package.json"), "utf8"));
    const version = packageJson.dependencies?.[name] ?? packageJson.devDependencies?.[name] ?? null;
    return { ecosystem: "node", name, installed: false, declaredVersion: version };
  }
}
async function dependencyRoot(name: string) {
  const dir = await safeExistingPath(`node_modules/${name}`); if (!(await fs.stat(dir)).isDirectory()) throw new Error(`Dependency not installed: ${name}`); return dir;
}

async function detectProject() {
  const exists = async (p: string) => fs.stat(path.join(root, p)).then(() => true).catch(() => false);
  let packageJson: any = null; try { packageJson = JSON.parse(await fs.readFile(path.join(root, "package.json"), "utf8")); } catch {}
  const deps = { ...(packageJson?.dependencies ?? {}), ...(packageJson?.devDependencies ?? {}) };
  const frameworks = ["next", "@nestjs/core", "react", "vue", "@angular/core", "express", "fastify"].filter((x) => x in deps);
  const manager = await exists("pnpm-lock.yaml") ? "pnpm" : await exists("yarn.lock") ? "yarn" : await exists("bun.lockb") ? "bun" : await exists("package-lock.json") ? "npm" : null;
  const languages = [await exists("tsconfig.json") ? "typescript" : null, await exists("pyproject.toml") || await exists("requirements.txt") ? "python" : null, await exists("Cargo.toml") ? "rust" : null, await exists("go.mod") ? "go" : null].filter(Boolean);
  const workspaces = packageJson?.workspaces ?? (await exists("pnpm-workspace.yaml") ? "pnpm-workspace.yaml" : null);
  return { packageJson, frameworks, packageManager: manager, languages, workspaces };
}

async function detectTests() {
  const p = await detectProject(); const scripts = p.packageJson?.scripts ?? {};
  const commands: string[] = [];
  for (const key of Object.keys(scripts)) if (/^(test|lint|typecheck|check|build)(:|$)/.test(key)) commands.push(`${p.packageManager ?? "npm"} ${p.packageManager === "npm" ? "run " : ""}${key}`);
  if (await fs.stat(path.join(root, "Cargo.toml")).then(() => true).catch(() => false)) commands.push("cargo test", "cargo check");
  if (await fs.stat(path.join(root, "pyproject.toml")).then(() => true).catch(() => false)) commands.push("pytest");
  if (await fs.stat(path.join(root, "go.mod")).then(() => true).catch(() => false)) commands.push("go test ./...");
  return [...new Set(commands)];
}

async function findRelatedTests(sourcePath: string) {
  const base = path.basename(sourcePath).replace(/\.[^.]+$/, "");
  const result = await searchCode(base, ".", 300, true, false);
  return result.matches.filter((m: string) => /(test|spec)\.[^:]+:/i.test(m) || /(__tests__|tests?)\//i.test(m)).slice(0, 100);
}

async function git(args: string[], timeoutMs = 20_000) { return runProcess("git", args, { cwd: root, timeoutMs }); }
async function currentHash(relativePath: string) { const file = await safeExistingPath(relativePath); return hashBuffer(await fs.readFile(file)); }

async function handleTool(tool: string, args: any) {
  if (tool === "project_info") {
    const detected = await detectProject(); const branch = await git(["branch", "--show-current"], 5000).catch(() => null); const instructions = await readInstructions(".");
    return { projectRoot: root, projectName: WORKSPACE_NAME, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, packageManager: detected.packageManager, languages: detected.languages, frameworks: detected.frameworks, workspaces: detected.workspaces, packageScripts: detected.packageJson?.scripts ?? null, gitBranch: branch?.stdout.trim() || null, gitignoreLoaded, shellEnabled: ALLOW_SHELL, dangerousShellEnabled: ALLOW_DANGEROUS, approvalMode: APPROVAL_MODE, instructions: instructions.instructionFiles, semantic: semantic.info(), capabilities: ["gitignore-aware-retrieval", "sensitive-path-policy", "range-read", "file-hash", "conflict-safe-edit", "dependency-inspection", "typescript-codegraph", "diagnostics", "test-intelligence", "git-history", "approval", "guarded-shell", "process-streaming", "multi-workspace-registration"] };
  }
  if (tool === "read_instructions") return readInstructions(args.path ?? ".");
  if (tool === "list_files") return listFiles(args.path ?? ".", args.maxDepth ?? 4, !!args.includeIgnored);
  if (tool === "file_info") { assertNotSensitive(args.path); return fileMeta(await safeExistingPath(args.path)); }
  if (tool === "read_file") return readOne(args.path);
  if (tool === "read_file_range") return readOne(args.path, Number(args.startLine), Number(args.endLine));
  if (tool === "read_files") { const files = []; let total = 0; for (const p of args.paths ?? []) { const f = await readOne(String(p)); total += f.size; if (total > MAX_BATCH_BYTES) throw new Error("Batch exceeds 8 MiB limit."); files.push(f); } return { files, totalBytes: total }; }
  if (tool === "search_code") return searchCode(String(args.query), args.path ?? ".", args.maxResults ?? 200, !!args.fixedStrings, !!args.includeIgnored);
  if (tool === "inspect_dependency") return inspectDependency(String(args.name));
  if (tool === "read_dependency") { const dir = await dependencyRoot(String(args.name)); const child = path.resolve(dir, String(args.path ?? "package.json")); if (!child.startsWith(dir + path.sep) && child !== dir) throw new Error("Dependency path escape."); const relative = rel(child); assertNotSensitive(relative); return readOne(relative, args.startLine, args.endLine); }
  if (tool === "search_dependency") { const dir = await dependencyRoot(String(args.name)); return searchCode(String(args.query), rel(dir), args.maxResults ?? 100, !!args.fixedStrings, true); }
  if (tool === "find_symbol") return { symbols: semantic.workspaceSymbols(String(args.query ?? ""), args.limit ?? 200) };
  if (tool === "find_definition") return { definitions: semantic.definitions(String(args.name), args.limit ?? 100) };
  if (tool === "find_references") return { references: semantic.references(String(args.name), args.limit ?? 500) };
  if (tool === "get_callers") return { callers: semantic.callers(String(args.name), args.limit ?? 300) };
  if (tool === "get_callees") return { callees: semantic.callees(String(args.name), args.limit ?? 300) };
  if (tool === "get_import_graph") return { edges: semantic.importGraph(args.limit ?? 2000) };
  if (tool === "get_diagnostics") return { engine: "typescript", diagnostics: semantic.diagnostics(args.limit ?? 500) };
  if (tool === "detect_test_commands") return { commands: await detectTests() };
  if (tool === "find_related_tests") return { matches: await findRelatedTests(String(args.path)) };
  if (tool === "run_affected_tests") { const matches = await findRelatedTests(String(args.path)); const detected = await detectProject(); const scripts = detected.packageJson?.scripts ?? {}; const testScript = Object.keys(scripts).find((k) => k === "test") ?? null; if (!testScript) return { started: false, reason: "No test script detected", related: matches }; const cmd = detected.packageManager === "npm" ? "npm test -- --runInBand" : `${detected.packageManager ?? "npm"} test`; return { related: matches, process: await startShell(cmd, ".", 1000, args.timeoutMs ?? 300000) }; }
  if (tool === "write_file") { const file = await safeWritePath(args.path); assertNotSensitive(rel(file)); if (args.expectedHash && await fs.stat(file).then(() => true).catch(() => false)) { const actual = await currentHash(rel(file)); if (actual !== args.expectedHash) throw new Error(`File changed since read. Expected ${args.expectedHash}, got ${actual}.`); } const content = String(args.content ?? ""); await fs.writeFile(file, content, "utf8"); semantic.invalidate(); return { ...(await fileMeta(file)), bytesWritten: Buffer.byteLength(content, "utf8") }; }
  if (tool === "edit_file") { const file = await safeExistingPath(args.path); assertNotSensitive(rel(file)); if (args.expectedHash) { const actual = await currentHash(rel(file)); if (actual !== args.expectedHash) throw new Error(`File changed since read. Expected ${args.expectedHash}, got ${actual}.`); } const original = await fs.readFile(file, "utf8"); const oldText = String(args.oldText); const count = original.split(oldText).length - 1; if (!count) throw new Error("oldText not found."); if (!args.replaceAll && count !== 1) throw new Error(`oldText occurs ${count} times.`); const updated = args.replaceAll ? original.split(oldText).join(String(args.newText ?? "")) : original.replace(oldText, String(args.newText ?? "")); await fs.writeFile(file, updated, "utf8"); semantic.invalidate(); return { ...(await fileMeta(file)), replacements: args.replaceAll ? count : 1 }; }
  if (tool === "apply_patch") { const patchText = String(args.patch ?? ""); validatePatchPaths(patchText); const check = await runProcess("git", ["apply", "--check", "--whitespace=nowarn", "-"], { cwd: root, input: patchText, timeoutMs: 20_000 }); if (check.exitCode !== 0) throw new Error(`Patch check failed:\n${check.stderr || check.stdout}`); const applied = await runProcess("git", ["apply", "--whitespace=nowarn", "-"], { cwd: root, input: patchText, timeoutMs: 20_000 }); if (applied.exitCode !== 0) throw new Error(`Patch failed:\n${applied.stderr || applied.stdout}`); semantic.invalidate(); return { applied: true }; }
  if (tool === "git_status") { const r = await git(["status", "--short", "--branch"]); return { exitCode: r.exitCode, output: r.stdout + r.stderr }; }
  if (tool === "git_diff") { const argv = ["diff", "--no-ext-diff", "--unified=3"]; if (args.cached) argv.push("--cached"); if (args.path) argv.push("--", rel(await safeExistingPath(String(args.path)))); const r = await git(argv); return { exitCode: r.exitCode, diff: (r.stdout + r.stderr).slice(-MAX_OUTPUT_BYTES) }; }
  if (tool === "git_log") { const r = await git(["log", `-${Math.min(args.limit ?? 20, 100)}`, "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s", ...(args.path ? ["--", String(args.path)] : [])]); return { output: r.stdout }; }
  if (tool === "git_show") { const r = await git(["show", "--stat", "--oneline", "--decorate", String(args.ref ?? "HEAD")]); return { output: (r.stdout + r.stderr).slice(-MAX_OUTPUT_BYTES) }; }
  if (tool === "git_blame") { assertNotSensitive(args.path); const r = await git(["blame", "--line-porcelain", ...(args.startLine && args.endLine ? ["-L", `${args.startLine},${args.endLine}`] : []), "--", String(args.path)]); return { output: r.stdout.slice(-MAX_OUTPUT_BYTES) }; }
  if (tool === "git_file_history") { assertNotSensitive(args.path); const r = await git(["log", "--follow", `-${Math.min(args.limit ?? 30, 100)}`, "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s", "--", String(args.path)]); return { output: r.stdout }; }
  if (tool === "run_command") return startShell(String(args.command), args.cwd ?? ".", args.yieldMs ?? 1000, args.timeoutMs ?? 0);
  if (tool === "process_poll") return processSnapshot(String(args.processId), typeof args.cursor === "number" ? args.cursor : undefined);
  if (tool === "process_list") return { processes: [...processes.entries()].map(([id, p]) => ({ processId: id, command: p.command, cwd: rel(p.cwd), running: p.exitCode === null, exitCode: p.exitCode, elapsedMs: Date.now() - p.startedAt })) };
  if (tool === "process_write") { const p = processes.get(String(args.processId)); if (!p || p.exitCode !== null) throw new Error("Process not running."); p.child.stdin.write(String(args.input ?? "")); return { written: true }; }
  if (tool === "process_kill") { const p = processes.get(String(args.processId)); if (!p) throw new Error("Unknown processId."); if (p.exitCode === null) p.child.kill(args.signal === "SIGKILL" ? "SIGKILL" : "SIGTERM"); return { killed: true }; }
  throw new Error(`Unknown tool: ${tool}`);
}

const watcher = chokidar.watch(root, { ignored: (p) => isSensitivePath(rel(p)) || rel(p).startsWith(".git/"), ignoreInitial: true, persistent: true });
watcher.on("all", (event, changed) => { const r = rel(changed); semantic.invalidate(); metadataEpoch++; if (r === ".gitignore") void reloadIgnore(); log("debug", "workspace.changed", { event, path: r, metadataEpoch }); });

let reconnectDelay = 1000;
function connect() {
  log("info", "client.connecting", { server: SERVER_URL, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, projectRoot: root });
  const ws = new WebSocket(SERVER_URL);
  ws.on("open", () => ws.send(JSON.stringify({ type: "register", token: DEVICE_TOKEN, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, workspaceName: WORKSPACE_NAME, projectRoot: root })));
  ws.on("message", async (raw) => {
    const msg = JSON.parse(raw.toString());
    if (msg.type === "registered") { reconnectDelay = 1000; log("info", "client.registered", { deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, projectRoot: root, shell: ALLOW_SHELL, approvalMode: APPROVAL_MODE }); return; }
    if (msg.type === "ping") { ws.send(JSON.stringify({ type: "pong", ts: Date.now(), deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID })); return; }
    if (msg.type !== "tool_call") return;
    const startedAt = Date.now(); log("info", "tool.received", { requestId: msg.id, tool: msg.tool, args: summarizeToolArgs(msg.tool, msg.args) });
    try { const result = await handleTool(msg.tool, msg.args ?? {}); ws.send(JSON.stringify({ type: "tool_result", id: msg.id, ok: true, result })); log("info", "tool.completed", { requestId: msg.id, tool: msg.tool, durationMs: Date.now() - startedAt }); }
    catch (error) { ws.send(JSON.stringify({ type: "tool_result", id: msg.id, ok: false, error: error instanceof Error ? error.message : String(error) })); log("error", "tool.failed", { requestId: msg.id, tool: msg.tool, durationMs: Date.now() - startedAt, error }); }
  });
  ws.on("close", () => { log("warn", "client.disconnected", { reconnectInMs: reconnectDelay }); setTimeout(connect, reconnectDelay); reconnectDelay = Math.min(reconnectDelay * 2, 30000); });
  ws.on("error", (error) => log("error", "client.socket_error", { error }));
}
log("info", "client.started", { version: VERSION, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, projectRoot: root, shell: ALLOW_SHELL, approvalMode: APPROVAL_MODE });
connect();
