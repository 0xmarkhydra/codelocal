import { promises as fs } from "node:fs";
import path from "node:path";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { randomUUID } from "node:crypto";
import WebSocket from "ws";

const SERVER_URL = process.env.SERVER_URL;
const DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const PROJECT_ROOT = process.env.PROJECT_ROOT;
const ALLOW_SHELL = process.env.CODELOCAL_ALLOW_SHELL === "1";
const ALLOW_DANGEROUS = process.env.CODELOCAL_ALLOW_DANGEROUS === "1";

const MAX_READ_BYTES = 1024 * 1024;
const MAX_BATCH_BYTES = 4 * 1024 * 1024;
const MAX_LIST_ENTRIES = 5000;
const MAX_OUTPUT_BYTES = 2 * 1024 * 1024;
const MAX_PROCESSES = 32;

if (!SERVER_URL || !DEVICE_TOKEN || !PROJECT_ROOT) {
  console.error("Required: SERVER_URL, DEVICE_TOKEN, PROJECT_ROOT");
  process.exit(1);
}

const root = await fs.realpath(path.resolve(PROJECT_ROOT));
const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;

function isInsideRoot(candidate: string) {
  return candidate === root || candidate.startsWith(rootPrefix);
}

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

function rel(p: string) {
  const value = path.relative(root, p);
  return value || ".";
}

async function listFiles(startRelative = ".", maxDepth = 4) {
  const start = await safeExistingPath(startRelative);
  if (!(await fs.stat(start)).isDirectory()) throw new Error("Path is not a directory.");
  const out: string[] = [];
  const skipped = new Set([".git", "node_modules", ".next", "dist", "build", "target", ".venv", "venv"]);

  async function walk(dir: string, depth: number) {
    if (out.length >= MAX_LIST_ENTRIES) return;
    const entries = (await fs.readdir(dir, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      if (out.length >= MAX_LIST_ENTRIES) break;
      const absolute = path.join(dir, entry.name);
      const display = rel(absolute);
      if (skipped.has(entry.name)) {
        out.push(`${display}/ [skipped]`);
        continue;
      }
      if (entry.isSymbolicLink()) {
        out.push(`${display} -> [symlink]`);
        continue;
      }
      if (entry.isDirectory()) {
        out.push(`${display}/`);
        if (depth < maxDepth) await walk(absolute, depth + 1);
      } else {
        out.push(display);
      }
    }
  }

  await walk(start, 0);
  return out;
}

function runProcess(command: string, args: string[], opts: { cwd?: string; input?: string; timeoutMs?: number } = {}) {
  return new Promise<{ stdout: string; stderr: string; exitCode: number | null }>((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: opts.cwd ?? root,
      env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CI: process.env.CI ?? "1" },
      stdio: "pipe",
    });
    let stdout = "";
    let stderr = "";
    let done = false;
    const append = (current: string, chunk: Buffer) => (current + chunk.toString()).slice(-MAX_OUTPUT_BYTES);
    child.stdout.on("data", (data) => { stdout = append(stdout, data); });
    child.stderr.on("data", (data) => { stderr = append(stderr, data); });
    child.on("error", reject);
    const timer = opts.timeoutMs
      ? setTimeout(() => { if (!done) child.kill("SIGTERM"); }, opts.timeoutMs)
      : null;
    child.on("close", (code) => {
      done = true;
      if (timer) clearTimeout(timer);
      resolve({ stdout, stderr, exitCode: code });
    });
    if (opts.input !== undefined) {
      child.stdin.write(opts.input);
      child.stdin.end();
    }
  });
}

async function readOne(requestedPath: string) {
  const file = await safeExistingPath(requestedPath);
  const stat = await fs.stat(file);
  if (!stat.isFile()) throw new Error(`${requestedPath}: not a file.`);
  if (stat.size > MAX_READ_BYTES) throw new Error(`${requestedPath}: exceeds ${MAX_READ_BYTES} byte read limit.`);
  return { path: rel(file), content: await fs.readFile(file, "utf8"), bytes: stat.size };
}

async function optionalText(relativePath: string, maxBytes = 128 * 1024) {
  try {
    const file = await safeExistingPath(relativePath);
    const stat = await fs.stat(file);
    if (!stat.isFile() || stat.size > maxBytes) return null;
    return await fs.readFile(file, "utf8");
  } catch {
    return null;
  }
}

async function searchCode(query: string, requestedPath = ".", maxResults = 200, fixedStrings = false) {
  const cwd = await safeExistingPath(requestedPath);
  if (!(await fs.stat(cwd)).isDirectory()) throw new Error("Search path is not a directory.");
  const rgArgs = [
    "--line-number", "--column", "--no-heading", "--color", "never", "--hidden",
    "--glob", "!.git/**", "--glob", "!node_modules/**", "--glob", "!.next/**",
    "--glob", "!dist/**", "--glob", "!build/**", "--glob", "!target/**",
  ];
  if (fixedStrings) rgArgs.push("--fixed-strings");
  rgArgs.push("--", query, ".");

  try {
    const result = await runProcess("rg", rgArgs, { cwd, timeoutMs: 20_000 });
    const all = result.stdout.split("\n").filter(Boolean);
    return { engine: "ripgrep", matches: all.slice(0, maxResults), truncated: all.length > maxResults };
  } catch {
    const grepArgs = ["-RIn", "--exclude-dir=.git", "--exclude-dir=node_modules", "--exclude-dir=.next", "--exclude-dir=dist", "--exclude-dir=build", "--", query, "."];
    const result = await runProcess("grep", grepArgs, { cwd, timeoutMs: 20_000 });
    const all = result.stdout.split("\n").filter(Boolean);
    return { engine: "grep", matches: all.slice(0, maxResults), truncated: all.length > maxResults };
  }
}

function validatePatchPaths(patchText: string) {
  for (const line of patchText.split("\n")) {
    if (!line.startsWith("+++ ") && !line.startsWith("--- ")) continue;
    const raw = line.slice(4).trim().split("\t")[0];
    if (raw === "/dev/null") continue;
    const normalized = raw.replace(/^[ab]\//, "");
    if (path.isAbsolute(normalized) || normalized.split(/[\\/]+/).includes("..")) {
      throw new Error(`Unsafe patch path: ${raw}`);
    }
    lexicalPath(normalized);
  }
}

function classifyCommand(command: string) {
  const normalized = command.replace(/\s+/g, " ").trim();
  const reasons: string[] = [];
  const blockedPatterns: Array<[RegExp, string]> = [
    [/(^|[;&|]\s*)sudo\b/i, "sudo is outside the project sandbox"],
    [/(^|[;&|]\s*)(su|ssh|scp|sftp)\b/i, "remote/session commands are blocked"],
    [/(^|[;&|]\s*)(shutdown|reboot|halt|launchctl|diskutil|mount|umount)\b/i, "system administration command"],
    [/\brm\s+(?:-[^\s]*r[^\s]*f|-[^\s]*f[^\s]*r)\s+(?:\/|~)(?:\s|$)/i, "recursive delete outside workspace"],
    [/\b(mkfs|fdisk|gpt)\b/i, "disk modification command"],
    [/\bdd\s+[^\n]*\bof=\/dev\//i, "raw device write"],
    [/\b(curl|wget)\b[^\n|;&]*\|\s*(sh|bash|zsh)\b/i, "download-and-execute pipeline"],
    [/(^|\s)\.\.\/(?:\.\.\/)?/, "parent-directory traversal in shell command"],
  ];
  for (const [pattern, reason] of blockedPatterns) {
    if (pattern.test(normalized)) reasons.push(reason);
  }
  return { allowed: reasons.length === 0 || ALLOW_DANGEROUS, reasons, normalized };
}

type ProcessRecord = {
  child: ChildProcessWithoutNullStreams;
  output: string;
  baseOffset: number;
  totalBytes: number;
  exitCode: number | null;
  startedAt: number;
  command: string;
  cwd: string;
};

const processes = new Map<string, ProcessRecord>();

function appendProcessOutput(id: string, chunk: Buffer, prefix = "") {
  const proc = processes.get(id);
  if (!proc) return;
  const text = prefix + chunk.toString();
  proc.totalBytes += Buffer.byteLength(text, "utf8");
  proc.output += text;
  if (Buffer.byteLength(proc.output, "utf8") > MAX_OUTPUT_BYTES) {
    const before = Buffer.byteLength(proc.output, "utf8");
    proc.output = proc.output.slice(Math.max(0, proc.output.length - MAX_OUTPUT_BYTES));
    const after = Buffer.byteLength(proc.output, "utf8");
    proc.baseOffset += before - after;
  }
}

function processSnapshot(id: string, cursor?: number) {
  const proc = processes.get(id);
  if (!proc) throw new Error("Unknown processId.");
  const requested = Math.max(cursor ?? proc.baseOffset, proc.baseOffset);
  const relativeOffset = Math.max(0, requested - proc.baseOffset);
  const output = proc.output.slice(relativeOffset);
  return {
    processId: id,
    running: proc.exitCode === null,
    exitCode: proc.exitCode,
    output,
    cursor: proc.totalBytes,
    truncatedBeforeCursor: (cursor ?? proc.baseOffset) < proc.baseOffset,
    elapsedMs: Date.now() - proc.startedAt,
    command: proc.command,
    cwd: rel(proc.cwd),
  };
}

function pruneProcesses() {
  if (processes.size < MAX_PROCESSES) return;
  const finished = [...processes.entries()]
    .filter(([, proc]) => proc.exitCode !== null)
    .sort((a, b) => a[1].startedAt - b[1].startedAt);
  while (processes.size >= MAX_PROCESSES && finished.length) {
    const [id] = finished.shift()!;
    processes.delete(id);
  }
  if (processes.size >= MAX_PROCESSES) throw new Error(`Too many active processes (${MAX_PROCESSES}). Stop one before starting another.`);
}

async function startShell(command: string, cwdRelative = ".", yieldMs = 1000, timeoutMs = 0) {
  if (!ALLOW_SHELL) throw new Error("Shell execution is disabled. Restart client with CODELOCAL_ALLOW_SHELL=1 to enable it.");
  const policy = classifyCommand(command);
  if (!policy.allowed) {
    throw new Error(`Command blocked by CodeLocal policy: ${policy.reasons.join("; ")}. Set CODELOCAL_ALLOW_DANGEROUS=1 only if you intentionally want unrestricted shell behavior.`);
  }

  const cwd = await safeExistingPath(cwdRelative);
  if (!(await fs.stat(cwd)).isDirectory()) throw new Error("cwd is not a directory.");
  pruneProcesses();

  const id = randomUUID();
  const child = spawn(command, {
    cwd,
    env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CI: process.env.CI ?? "1" },
    shell: true,
    stdio: "pipe",
  }) as ChildProcessWithoutNullStreams;

  processes.set(id, {
    child,
    output: "",
    baseOffset: 0,
    totalBytes: 0,
    exitCode: null,
    startedAt: Date.now(),
    command,
    cwd,
  });

  child.stdout.on("data", (data) => appendProcessOutput(id, data));
  child.stderr.on("data", (data) => appendProcessOutput(id, data, "[stderr] "));
  child.on("close", (code) => {
    const proc = processes.get(id);
    if (proc) proc.exitCode = code;
  });
  child.on("error", (error) => {
    const proc = processes.get(id);
    if (proc) {
      appendProcessOutput(id, Buffer.from(`\n[process error] ${error.message}\n`));
      proc.exitCode = -1;
    }
  });
  if (timeoutMs > 0) {
    setTimeout(() => {
      const proc = processes.get(id);
      if (proc && proc.exitCode === null) {
        appendProcessOutput(id, Buffer.from(`\n[CodeLocal] timeout after ${timeoutMs}ms; sending SIGTERM\n`));
        proc.child.kill("SIGTERM");
      }
    }, timeoutMs);
  }

  await new Promise((resolve) => setTimeout(resolve, Math.min(Math.max(yieldMs, 0), 10_000)));
  return processSnapshot(id, 0);
}

async function readInstructions(requestedPath = ".") {
  const target = await safeExistingPath(requestedPath);
  const stat = await fs.stat(target);
  const targetDir = stat.isDirectory() ? target : path.dirname(target);
  const dirs: string[] = [];
  let current = targetDir;
  while (isInsideRoot(current)) {
    dirs.push(current);
    if (current === root) break;
    current = path.dirname(current);
  }
  dirs.reverse();

  const files: Array<{ path: string; content: string }> = [];
  for (const dir of dirs) {
    const relativeDir = rel(dir);
    const candidate = relativeDir === "." ? "AGENTS.md" : path.posix.join(relativeDir.split(path.sep).join("/"), "AGENTS.md");
    const content = await optionalText(candidate);
    if (content) files.push({ path: candidate, content });
  }
  if (dirs[0] === root) {
    for (const candidate of ["CLAUDE.md", ".github/copilot-instructions.md"]) {
      const content = await optionalText(candidate);
      if (content) files.push({ path: candidate, content });
    }
  }
  return { requestedPath, instructionFiles: files };
}

async function handleTool(tool: string, args: any) {
  if (tool === "project_info") {
    const packageJsonText = await optionalText("package.json");
    let packageJson: any = null;
    try { packageJson = packageJsonText ? JSON.parse(packageJsonText) : null; } catch { packageJson = null; }

    const gitRoot = await runProcess("git", ["rev-parse", "--show-toplevel"], { cwd: root, timeoutMs: 5000 }).catch(() => null);
    const branch = await runProcess("git", ["branch", "--show-current"], { cwd: root, timeoutMs: 5000 }).catch(() => null);
    const instructions = await readInstructions(".");
    const lockfileCandidates = ["pnpm-lock.yaml", "yarn.lock", "package-lock.json", "bun.lockb", "uv.lock", "poetry.lock", "Cargo.lock", "go.sum"];
    const lockfiles = (await Promise.all(lockfileCandidates.map(async (file) => [file, !!(await optionalText(file, 1024 * 1024))] as const)))
      .filter(([, exists]) => exists)
      .map(([file]) => file);

    const toolchains: string[] = [];
    if (packageJsonText) toolchains.push("node");
    if (await optionalText("Cargo.toml")) toolchains.push("rust");
    if (await optionalText("pyproject.toml") || await optionalText("requirements.txt")) toolchains.push("python");
    if (await optionalText("go.mod")) toolchains.push("go");

    return {
      projectRoot: root,
      projectName: packageJson?.name ?? path.basename(root),
      packageScripts: packageJson?.scripts ?? null,
      lockfiles,
      toolchains,
      gitRepository: !!gitRoot && gitRoot.exitCode === 0,
      gitBranch: branch?.stdout.trim() || null,
      shellEnabled: ALLOW_SHELL,
      dangerousShellEnabled: ALLOW_DANGEROUS,
      instructions: instructions.instructionFiles,
      capabilities: ["filesystem", "batch-read", "search", "instructions", "patch", "git", "shell-policy", "process-streaming"],
    };
  }

  if (tool === "read_instructions") return readInstructions(args.path ?? ".");
  if (tool === "list_files") return { files: await listFiles(args.path ?? ".", args.maxDepth ?? 4) };
  if (tool === "read_file") return readOne(args.path);
  if (tool === "read_files") {
    const paths = Array.isArray(args.paths) ? args.paths : [];
    if (paths.length > 50) throw new Error("read_files supports at most 50 files per call.");
    const files = [];
    let total = 0;
    for (const requested of paths) {
      const file = await readOne(String(requested));
      total += file.bytes;
      if (total > MAX_BATCH_BYTES) throw new Error("Batch exceeds 4 MiB limit.");
      files.push(file);
    }
    return { files, totalBytes: total };
  }
  if (tool === "search_code") return searchCode(String(args.query), args.path ?? ".", args.maxResults ?? 200, !!args.fixedStrings);

  if (tool === "write_file") {
    const file = await safeWritePath(args.path);
    const content = String(args.content ?? "");
    await fs.writeFile(file, content, "utf8");
    return { path: rel(file), bytes: Buffer.byteLength(content, "utf8") };
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
    const result = await runProcess("git", ["status", "--short", "--branch"], { cwd: root, timeoutMs: 10_000 });
    return { exitCode: result.exitCode, output: result.stdout + result.stderr };
  }

  if (tool === "git_diff") {
    const argv = ["diff", "--no-ext-diff", "--unified=3"];
    if (args.cached) argv.push("--cached");
    if (args.path) {
      const safe = await safeExistingPath(String(args.path));
      argv.push("--", rel(safe));
    }
    const result = await runProcess("git", argv, { cwd: root, timeoutMs: 20_000 });
    return { exitCode: result.exitCode, diff: (result.stdout + result.stderr).slice(-MAX_OUTPUT_BYTES) };
  }

  if (tool === "run_command") return startShell(String(args.command), args.cwd ?? ".", args.yieldMs ?? 1000, args.timeoutMs ?? 0);
  if (tool === "process_poll") return processSnapshot(String(args.processId), typeof args.cursor === "number" ? args.cursor : undefined);
  if (tool === "process_list") {
    return {
      processes: [...processes.entries()].map(([id, proc]) => ({
        processId: id,
        command: proc.command,
        cwd: rel(proc.cwd),
        running: proc.exitCode === null,
        exitCode: proc.exitCode,
        elapsedMs: Date.now() - proc.startedAt,
      })),
    };
  }
  if (tool === "process_write") {
    const proc = processes.get(String(args.processId));
    if (!proc) throw new Error("Unknown processId.");
    if (proc.exitCode !== null) throw new Error("Process already exited.");
    proc.child.stdin.write(String(args.input ?? ""));
    return { written: true };
  }
  if (tool === "process_kill") {
    const proc = processes.get(String(args.processId));
    if (!proc) throw new Error("Unknown processId.");
    if (proc.exitCode === null) proc.child.kill(args.signal === "SIGKILL" ? "SIGKILL" : "SIGTERM");
    return { killed: true };
  }

  throw new Error(`Unknown tool: ${tool}`);
}

function connect() {
  const ws = new WebSocket(SERVER_URL);
  ws.on("open", () => ws.send(JSON.stringify({ type: "register", token: DEVICE_TOKEN })));
  ws.on("message", async (raw) => {
    const msg = JSON.parse(raw.toString());
    if (msg.type === "registered") {
      console.log(`Connected. PROJECT_ROOT=${root}`);
      console.log(`Shell: ${ALLOW_SHELL ? "ENABLED" : "disabled (set CODELOCAL_ALLOW_SHELL=1)"}`);
      console.log(`Dangerous shell: ${ALLOW_DANGEROUS ? "ENABLED" : "blocked by policy"}`);
      return;
    }
    if (msg.type !== "tool_call") return;
    try {
      const result = await handleTool(msg.tool, msg.args ?? {});
      ws.send(JSON.stringify({ type: "tool_result", id: msg.id, ok: true, result }));
    } catch (error) {
      ws.send(JSON.stringify({ type: "tool_result", id: msg.id, ok: false, error: error instanceof Error ? error.message : String(error) }));
    }
  });
  ws.on("close", () => {
    console.log("Disconnected; reconnecting...");
    setTimeout(connect, 2000);
  });
  ws.on("error", (error) => console.error("WebSocket error:", error.message));
}

connect();
