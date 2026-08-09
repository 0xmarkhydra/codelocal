import { promises as fs } from "node:fs";
import path from "node:path";
import os from "node:os";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import WebSocket from "ws";
import ignore, { type Ignore } from "ignore";
import chokidar from "chokidar";
import { log, mirrorProcessOutput, summarizeToolArgs } from "./log.js";
import { PROTOCOL_VERSION, isSideEffectingTool, normalizeError, type ToolCallMessage } from "./protocol.js";
import { classifyCommand, classifyGitWrite, isSensitivePath, type NetworkPolicy } from "./security-policy.js";
import { SandboxManager } from "./sandbox.js";
import { ProcessManager } from "./process-manager.js";
import { IdempotencyJournal } from "./state.js";
import { type ApprovalMode } from "./approval.js";
import { ChatApprovalBroker } from "./chat-approval.js";
import { TerminalHistory } from "./terminal-history.js";
import { audit } from "./audit.js";
import { SemanticRouter } from "./semantic-router.js";
import { ProjectContextEngine } from "./context-engine.js";
import { EditingEngine } from "./editing-engine.js";
import { VerificationEngine } from "./verification.js";
import { defaultDeviceIdentity, loadLocalCredential } from "./identity.js";
import { McpHub } from "./mcp-hub.js";

const SERVER_URL = process.env.SERVER_URL;
const PROJECT_ROOT = process.env.PROJECT_ROOT;
const LEGACY_DEVICE_TOKEN = process.env.DEVICE_TOKEN;
const ALLOW_SHELL = process.env.CODELOCAL_ALLOW_SHELL === "1";
const APPROVAL_MODE = ((process.env.CODELOCAL_APPROVAL_MODE ?? "prompt") as ApprovalMode);
const NETWORK_POLICY = ((process.env.CODELOCAL_NETWORK ?? "approval") as NetworkPolicy);
const MIRROR_PROCESS_OUTPUT = process.env.CODELOCAL_MIRROR_PROCESS_OUTPUT !== "0";
const MAX_READ_BYTES = Number(process.env.CODELOCAL_MAX_READ_BYTES ?? 2 * 1024 * 1024);
const MAX_BATCH_BYTES = Number(process.env.CODELOCAL_MAX_BATCH_BYTES ?? 8 * 1024 * 1024);
const MAX_LIST_ENTRIES = Number(process.env.CODELOCAL_MAX_LIST_ENTRIES ?? 10000);
const MAX_OUTPUT_BYTES = Number(process.env.CODELOCAL_MAX_OUTPUT_BYTES ?? 2 * 1024 * 1024);

if (!SERVER_URL || !PROJECT_ROOT) {
  console.error("Required: SERVER_URL and PROJECT_ROOT. Pairing credential or DEVICE_TOKEN is also required for registration.");
  process.exit(1);
}

const root = await fs.realpath(path.resolve(PROJECT_ROOT));
const rootPrefix = root.endsWith(path.sep) ? root : root + path.sep;
const localCredential = await loadLocalCredential(SERVER_URL);
const defaults = defaultDeviceIdentity();
const DEVICE_ID = process.env.CODELOCAL_DEVICE_ID ?? localCredential?.deviceId ?? defaults.deviceId;
const DEVICE_NAME = process.env.CODELOCAL_DEVICE_NAME ?? localCredential?.deviceName ?? defaults.deviceName;
const WORKSPACE_ID = process.env.CODELOCAL_WORKSPACE_ID ?? path.basename(root);
const WORKSPACE_NAME = process.env.CODELOCAL_WORKSPACE_NAME ?? path.basename(root);
const WORKSPACE_KEY = `${DEVICE_ID}::${WORKSPACE_ID}`;

const semantic = new SemanticRouter(root);
const context = new ProjectContextEngine(root, semantic);
const editing = new EditingEngine(root);
const verification = new VerificationEngine(root, semantic, context);
const sandbox = new SandboxManager(root, NETWORK_POLICY);
const chatApproval = new ChatApprovalBroker();
const terminalHistory = new TerminalHistory();
const journal = new IdempotencyJournal();
let mcpConnectAuthorized = false;
const mcpHub = new McpHub(root, async () => {
  if (!mcpConnectAuthorized) throw new Error("Starting an installed MCP runtime requires approval in ChatGPT. Call mcp_call and approve it there.");
});
const processManager = new ProcessManager(root, WORKSPACE_KEY, sandbox, (_record, stream, text) => {
  if (MIRROR_PROCESS_OUTPUT) mirrorProcessOutput(stream, text);
}, async (record) => {
  await terminalHistory.finished(record);
  semantic.invalidate();
  context.invalidate();
  metadataEpoch++;
  log("debug", "workspace.process_settled", { metadataEpoch });
});

let ignoreMatcher: Ignore = ignore();
let metadataEpoch = 0;
let reconnectDelay = 1000;
let activeSocket: WebSocket | null = null;

function isInsideRoot(candidate: string) {
  return candidate === root || candidate.startsWith(rootPrefix);
}

function rel(p: string) {
  return path.relative(root, p).split(path.sep).join("/") || ".";
}

function lexicalPath(relativePath: string) {
  if (path.isAbsolute(relativePath)) throw new Error("Absolute paths are not allowed.");
  const candidate = path.resolve(root, relativePath || ".");
  if (!isInsideRoot(candidate)) throw new Error("Path escapes PROJECT_ROOT.");
  return candidate;
}

async function safeExistingPath(relativePath: string) {
  if (isSensitivePath(relativePath)) throw new Error(`Access blocked by sensitive-path policy: ${relativePath}`);
  const real = await fs.realpath(lexicalPath(relativePath));
  if (!isInsideRoot(real)) throw new Error("Resolved path escapes PROJECT_ROOT (possible symlink traversal).");
  return real;
}

async function safeWritePath(relativePath: string) {
  if (isSensitivePath(relativePath)) throw new Error(`Access blocked by sensitive-path policy: ${relativePath}`);
  const candidate = lexicalPath(relativePath);
  await fs.mkdir(path.dirname(candidate), { recursive: true });
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

async function reloadIgnore() {
  const matcher = ignore();
  matcher.add([".git/", ".DS_Store"]);
  try { matcher.add(await fs.readFile(path.join(root, ".gitignore"), "utf8")); } catch {}
  try { matcher.add(await fs.readFile(path.join(root, ".git", "info", "exclude"), "utf8")); } catch {}
  ignoreMatcher = matcher;
  metadataEpoch++;
}
await reloadIgnore();

function isIgnored(relativePath: string) {
  const p = relativePath.replace(/\\/g, "/").replace(/^\.\//, "");
  if (!p || p === ".") return false;
  return ignoreMatcher.ignores(p) || ignoreMatcher.ignores(p.endsWith("/") ? p : `${p}/`);
}

function hashBuffer(buf: Buffer) {
  return createHash("sha256").update(buf).digest("hex");
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

async function fileMeta(file: string) {
  const stat = await fs.stat(file);
  const relative = rel(file);
  if (isSensitivePath(relative)) throw new Error(`Access blocked by sensitive-path policy: ${relative}`);
  const hash = stat.isFile() && stat.size <= MAX_READ_BYTES * 4 ? hashBuffer(await fs.readFile(file)) : null;
  return { path: relative, size: stat.size, mtimeMs: stat.mtimeMs, hash, ignored: isIgnored(relative), isFile: stat.isFile(), isDirectory: stat.isDirectory() };
}

async function readOne(requestedPath: string, startLine?: number, endLine?: number) {
  const file = await safeExistingPath(requestedPath);
  const stat = await fs.stat(file);
  if (!stat.isFile()) throw new Error(`${requestedPath}: not a file.`);
  if (stat.size > MAX_READ_BYTES && startLine == null) throw new Error(`${requestedPath}: exceeds read limit; use read_file_range.`);
  if (await isBinary(file)) return { ...(await fileMeta(file)), binary: true, content: null };
  const text = await fs.readFile(file, "utf8");
  const lines = text.split(/\r?\n/);
  const from = Math.max(1, startLine ?? 1);
  const to = Math.min(lines.length, endLine ?? lines.length);
  return { ...(await fileMeta(file)), binary: false, startLine: from, endLine: to, totalLines: lines.length, content: lines.slice(from - 1, to).join("\n") };
}

async function listFiles(startRelative = ".", maxDepth = 4, includeIgnored = false) {
  const start = await safeExistingPath(startRelative);
  if (!(await fs.stat(start)).isDirectory()) throw new Error("Path is not a directory.");
  const entries: Array<{ path: string; type: "file" | "directory" | "symlink"; ignored?: boolean; sensitive?: boolean }> = [];
  const walk = async (dir: string, depth: number) => {
    if (entries.length >= MAX_LIST_ENTRIES) return;
    const children = await fs.readdir(dir, { withFileTypes: true });
    children.sort((a, b) => a.name.localeCompare(b.name));
    for (const child of children) {
      if (entries.length >= MAX_LIST_ENTRIES) break;
      const absolute = path.join(dir, child.name);
      const relative = rel(absolute);
      const sensitive = isSensitivePath(relative);
      const ignored = isIgnored(relative);
      if (sensitive) { entries.push({ path: relative, type: child.isDirectory() ? "directory" : "file", sensitive: true }); continue; }
      if (ignored && !includeIgnored) continue;
      if (child.isSymbolicLink()) { entries.push({ path: relative, type: "symlink", ignored }); continue; }
      if (child.isDirectory()) {
        entries.push({ path: `${relative}/`, type: "directory", ignored });
        if (depth < maxDepth) await walk(absolute, depth + 1);
      } else entries.push({ path: relative, type: "file", ignored });
    }
  };
  await walk(start, 0);
  return { entries, truncated: entries.length >= MAX_LIST_ENTRIES, includeIgnored };
}

async function runDirect(command: string, args: string[], options: { cwd?: string; input?: string; timeoutMs?: number } = {}) {
  return new Promise<{ stdout: string; stderr: string; exitCode: number | null }>((resolve, reject) => {
    const child = spawn(command, args, { cwd: options.cwd ?? root, env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CI: process.env.CI ?? "1" }, stdio: "pipe" });
    let stdout = "", stderr = "", done = false;
    const timer = options.timeoutMs ? setTimeout(() => { if (!done) child.kill("SIGTERM"); }, options.timeoutMs) : null;
    child.stdout.on("data", (d) => { stdout = (stdout + d.toString()).slice(-MAX_OUTPUT_BYTES); });
    child.stderr.on("data", (d) => { stderr = (stderr + d.toString()).slice(-MAX_OUTPUT_BYTES); });
    child.on("error", reject);
    child.on("close", (code) => { done = true; if (timer) clearTimeout(timer); resolve({ stdout, stderr, exitCode: code }); });
    if (options.input != null) { child.stdin.write(options.input); child.stdin.end(); }
  });
}

async function searchCode(query: string, requestedPath = ".", maxResults = 200, fixedStrings = false, includeIgnored = false) {
  const cwd = await safeExistingPath(requestedPath);
  const rgArgs = ["--line-number", "--column", "--no-heading", "--color", "never", "--hidden"];
  if (includeIgnored) rgArgs.push("--no-ignore");
  if (fixedStrings) rgArgs.push("--fixed-strings");
  rgArgs.push("--glob", "!.git/**", "--", query, ".");
  let result = await runDirect("rg", rgArgs, { cwd, timeoutMs: 30_000 }).catch(() => null);
  if (!result) result = await runDirect("grep", ["-RIn", "--", query, "."], { cwd, timeoutMs: 30_000 });
  const matches = result.stdout.split("\n").filter(Boolean).filter((line) => !isSensitivePath(line.split(":", 1)[0].replace(/^\.\//, "")));
  return { matches: matches.slice(0, maxResults), truncated: matches.length > maxResults, includeIgnored };
}

async function git(args: string[], timeoutMs = 30_000) {
  return runDirect("git", args, { cwd: root, timeoutMs });
}

async function readInstructions(requestedPath = ".") {
  const target = await safeExistingPath(requestedPath);
  const stat = await fs.stat(target);
  const targetDir = stat.isDirectory() ? target : path.dirname(target);
  const dirs: string[] = [];
  let current = targetDir;
  while (isInsideRoot(current)) { dirs.push(current); if (current === root) break; current = path.dirname(current); }
  dirs.reverse();
  const instructionFiles: Array<{ path: string; content: string }> = [];
  for (const dir of dirs) {
    const candidate = rel(dir) === "." ? "AGENTS.md" : `${rel(dir)}/AGENTS.md`;
    try { const r = await readOne(candidate); if (!r.binary && r.content) instructionFiles.push({ path: candidate, content: r.content }); } catch {}
  }
  for (const candidate of ["CLAUDE.md", ".github/copilot-instructions.md"]) {
    try { const r = await readOne(candidate); if (!r.binary && r.content) instructionFiles.push({ path: candidate, content: r.content }); } catch {}
  }
  return { requestedPath, instructionFiles };
}

async function inspectDependency(name: string, ecosystem = "auto") {
  const project = await context.map();
  const chosen = ecosystem === "auto" ? (project.languages.includes("python") && !project.languages.includes("typescript/javascript") ? "python" : project.languages.includes("rust") && !project.languages.includes("typescript/javascript") ? "rust" : project.languages.includes("go") && !project.languages.includes("typescript/javascript") ? "go" : "node") : ecosystem;
  if (chosen === "node") {
    const pkg = path.join(root, "node_modules", name, "package.json");
    try { const parsed = JSON.parse(await fs.readFile(pkg, "utf8")); return { ecosystem: "node", installed: true, name: parsed.name ?? name, version: parsed.version ?? null, packagePath: rel(pkg), exports: parsed.exports ?? null, types: parsed.types ?? parsed.typings ?? null, dependencies: parsed.dependencies ?? null }; }
    catch { return { ecosystem: "node", installed: false, name }; }
  }
  if (chosen === "python") {
    const r = await runDirect(process.platform === "win32" ? "python" : "python3", ["-c", `import importlib.util,json; s=importlib.util.find_spec(${JSON.stringify(name)}); print(json.dumps({\"found\":bool(s),\"origin\":getattr(s,\"origin\",None),\"locations\":list(getattr(s,\"submodule_search_locations\",[]) or [])}))`], { timeoutMs: 10_000 }).catch(() => null);
    return { ecosystem: "python", name, ...(r ? JSON.parse(r.stdout || "{}") : { found: false }) };
  }
  if (chosen === "rust") {
    const r = await runDirect("cargo", ["metadata", "--format-version", "1", "--no-deps"], { timeoutMs: 30_000 });
    const metadata = JSON.parse(r.stdout || "{}");
    const pkg = (metadata.packages ?? []).find((p: any) => p.name === name);
    return { ecosystem: "rust", name, package: pkg ?? null };
  }
  if (chosen === "go") {
    const r = await runDirect("go", ["list", "-m", "-json", name], { timeoutMs: 20_000 });
    return { ecosystem: "go", name, module: r.exitCode === 0 ? JSON.parse(r.stdout || "{}") : null };
  }
  return { ecosystem: chosen, name, supported: false };
}

function validatePatchPaths(patchText: string) {
  for (const line of patchText.split("\n")) {
    if (!line.startsWith("+++ ") && !line.startsWith("--- ")) continue;
    const raw = line.slice(4).trim().split("\t")[0];
    if (raw === "/dev/null") continue;
    const normalized = raw.replace(/^[ab]\//, "");
    if (path.isAbsolute(normalized) || normalized.split(/[\\/]+/).includes("..")) throw new Error(`Unsafe patch path: ${raw}`);
    if (isSensitivePath(normalized)) throw new Error(`Access blocked by sensitive-path policy: ${normalized}`);
    lexicalPath(normalized);
  }
}

async function commandCwd(requested = ".") {
  const cwd = await safeExistingPath(requested);
  if (!(await fs.stat(cwd)).isDirectory()) throw new Error("Command cwd is not a directory.");
  return cwd;
}

async function terminalPreflight(command: string, requestedCwd = ".", requestId?: string) {
  if (!ALLOW_SHELL) return { status: "blocked", riskLevel: "BLOCKED", reason: "Shell execution is disabled for this workspace.", matchedRules: ["shell-disabled"], command: String(command) };
  const cwd = await commandCwd(requestedCwd);
  const decision = classifyCommand(command, NETWORK_POLICY);
  const result = chatApproval.preflight(command, rel(cwd), decision);
  await audit({ event: "terminal.preflight", requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, detail: { cwd: rel(cwd), command: decision.redactedCommand, rules: decision.matchedRules, status: result.status } });
  return { ...result, cwd: rel(cwd) };
}

async function runGuardedCommand(command: string, options: { cwd?: string; timeoutMs?: number; usePty?: boolean; requestId?: string; ownerSessionId?: string; approvalToken?: string } = {}) {
  if (!ALLOW_SHELL) throw new Error("Shell execution is disabled. Restart client with CODELOCAL_ALLOW_SHELL=1.");
  const cwd = await commandCwd(options.cwd ?? ".");
  const relativeCwd = rel(cwd);
  const decision = classifyCommand(command, NETWORK_POLICY);
  await audit({ event: "policy.command", requestId: options.requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, detail: { cwd: relativeCwd, command: decision.redactedCommand, rules: decision.matchedRules, blocked: decision.blocked } });
  if (decision.blocked) throw new Error(`Command blocked by policy: ${decision.reason}`);

  let approvalMode: "automatic" | "chat" = "automatic";
  if (decision.requiresApproval) {
    const approved = chatApproval.consume(options.approvalToken, command, relativeCwd, decision);
    if (!approved) {
      const pending = chatApproval.preflight(command, relativeCwd, decision);
      await audit({ event: "terminal.approval_required", requestId: options.requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, detail: { cwd: relativeCwd, command: decision.redactedCommand, rules: decision.matchedRules, expiresAt: pending.expiresAt } });
      return { ...pending, cwd: relativeCwd };
    }
    approvalMode = "chat";
    await audit({ event: "terminal.chat_approved", requestId: options.requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, status: "approved", detail: { cwd: relativeCwd, command: decision.redactedCommand, rules: decision.matchedRules } });
  }

  const started = await processManager.start(command, { cwd, timeoutMs: options.timeoutMs, usePty: options.usePty, requestId: options.requestId, ownerSessionId: options.ownerSessionId });
  await terminalHistory.started({
    workspaceKey: WORKSPACE_KEY,
    processId: started.processId,
    requestId: options.requestId,
    sessionId: options.ownerSessionId,
    cwd: relativeCwd,
    command,
    riskLevel: decision.riskLevel,
    matchedRules: decision.matchedRules,
    approval: approvalMode,
    startedAt: started.startedAt,
    executionMode: started.executionMode,
  });
  return started;
}

async function gitWrite(operation: string, detail: string, args: string[], requestId?: string, approvalToken?: string) {
  const decision = classifyGitWrite(operation, detail);
  if (decision.blocked) throw new Error(`Git operation blocked by policy: ${decision.reason}`);
  const approvalCommand = `git ${operation} ${detail}`.trim();
  if (decision.requiresApproval) {
    const approved = chatApproval.consume(approvalToken, approvalCommand, ".", decision);
    if (!approved) {
      const pending = chatApproval.preflight(approvalCommand, ".", decision);
      await audit({ event: "git.approval_required", requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, detail: { operation, command: decision.redactedCommand, rules: decision.matchedRules, expiresAt: pending.expiresAt } });
      return pending;
    }
    await audit({ event: "git.chat_approved", requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, status: "approved", detail: { operation, command: decision.redactedCommand, rules: decision.matchedRules } });
  }
  const result = await git(args, 120_000);
  if (result.exitCode !== 0) throw new Error(result.stderr || result.stdout || `git ${operation} failed`);
  await audit({ event: "git.write", requestId, workspaceKey: WORKSPACE_KEY, status: "completed", detail: { operation, outputBytes: Buffer.byteLength(result.stdout + result.stderr, "utf8") } });
  return { exitCode: result.exitCode, output: result.stdout + result.stderr };
}

async function callInstalledMcp(server: string, tool: string, args: Record<string, unknown>, requestId?: string, approvalToken?: string) {
  const info = await mcpHub.toolInfo(server, tool);
  const readOnlyHint = info.annotations?.readOnlyHint === true;
  const rule = `mcp:${server}:${tool}`;
  const decision = {
    riskLevel: "REVIEW" as const,
    matchedRules: [rule],
    requiresApproval: true,
    blocked: false,
    redactedCommand: `mcp ${server}.${tool}`,
    reason: readOnlyHint
      ? "external MCP call; server advertises readOnlyHint, but external annotations are advisory"
      : "external MCP call may have side effects",
  };
  await audit({ event: "policy.mcp", requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, detail: { server, tool, readOnlyHint, rule } });
  const approvalCommand = `mcp ${server}.${tool}`;
  const approved = chatApproval.consume(approvalToken, approvalCommand, ".", decision);
  if (!approved) {
    const pending = chatApproval.preflight(approvalCommand, ".", decision);
    await audit({ event: "mcp.approval_required", requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, detail: { server, tool, rule, expiresAt: pending.expiresAt } });
    return pending;
  }
  await audit({ event: "mcp.chat_approved", requestId, workspaceKey: WORKSPACE_KEY, riskLevel: decision.riskLevel, status: "approved", detail: { server, tool, rule } });
  const startedAt = Date.now();
  mcpConnectAuthorized = true;
  try {
    const result = await mcpHub.callTool(server, tool, args);
    await audit({ event: "mcp.call", requestId, workspaceKey: WORKSPACE_KEY, tool: `${server}.${tool}`, status: "ok", detail: { durationMs: Date.now() - startedAt } });
    return result;
  } catch (error) {
    await audit({ event: "mcp.call", requestId, workspaceKey: WORKSPACE_KEY, tool: `${server}.${tool}`, status: "failed", detail: { durationMs: Date.now() - startedAt, error: error instanceof Error ? error.message : String(error) } });
    throw error;
  } finally {
    mcpConnectAuthorized = false;
  }
}

async function handleTool(tool: string, args: any, request: { requestId: string; sessionId?: string }) {
  if (tool === "project_info") {
    const project = await context.map();
    return {
      protocolVersion: PROTOCOL_VERSION,
      projectRoot: root,
      projectName: WORKSPACE_NAME,
      deviceId: DEVICE_ID,
      deviceName: DEVICE_NAME,
      workspaceId: WORKSPACE_ID,
      workspaceKey: WORKSPACE_KEY,
      project,
      instructions: (await readInstructions(".")).instructionFiles,
      semantic: await semantic.info(),
      sandbox: await sandbox.info(),
      shellEnabled: ALLOW_SHELL,
      approvalMode: APPROVAL_MODE,
      terminalApproval: "chat-mediated",
      networkPolicy: NETWORK_POLICY,
      metadataEpoch,
      capabilities: ["protocol-v2", "gitignore-aware-retrieval", "sensitive-path-policy", "polyglot-semantic-router", "lsp", "context-engine", "transactional-edits", "diagnostic-regression", "process-manager-v2", "pty-when-installed", "cancellation", "sandbox", "idempotency", "git-write-approval", "terminal-chat-approval", "terminal-history", "mcp-hub", "audit"],
    };
  }
  if (tool === "mcp_list") return { servers: await mcpHub.listServers() };
  if (tool === "mcp_search_tools") return mcpHub.searchTools(String(args.query ?? ""), { limit: args.limit ?? 8, server: args.server, refresh: !!args.refresh });
  if (tool === "mcp_tool_info") return mcpHub.toolInfo(String(args.server), String(args.tool));
  if (tool === "mcp_call") return callInstalledMcp(String(args.server), String(args.tool), (args.arguments ?? {}) as Record<string, unknown>, request.requestId, args.approvalToken);
  if (tool === "read_instructions") return readInstructions(args.path ?? ".");
  if (tool === "project_map") return context.map(!!args.force);
  if (tool === "context_for_task") return context.relevant(String(args.taskHint ?? ""), args.limit ?? 30);
  if (tool === "list_files") return listFiles(args.path ?? ".", args.maxDepth ?? 4, !!args.includeIgnored);
  if (tool === "file_info") return fileMeta(await safeExistingPath(String(args.path)));
  if (tool === "read_file") return readOne(String(args.path));
  if (tool === "read_file_range") return readOne(String(args.path), Number(args.startLine), Number(args.endLine));
  if (tool === "read_files") {
    const files = []; let total = 0;
    for (const p of args.paths ?? []) { const f = await readOne(String(p)); total += Number(f.size ?? 0); if (total > MAX_BATCH_BYTES) throw new Error("Batch exceeds read limit."); files.push(f); }
    return { files, totalBytes: total };
  }
  if (tool === "search_code") return searchCode(String(args.query), args.path ?? ".", args.maxResults ?? 200, !!args.fixedStrings, !!args.includeIgnored);
  if (tool === "inspect_dependency") return inspectDependency(String(args.name), args.ecosystem ?? "auto");
  if (tool === "read_dependency") {
    const project = await context.map();
    if (args.ecosystem && args.ecosystem !== "node") throw new Error("Targeted read_dependency currently uses filesystem paths for installed Node dependencies; use inspect_dependency plus read_file for other ecosystems.");
    const base = await fs.realpath(path.join(root, "node_modules", String(args.name)));
    const target = path.resolve(base, String(args.path ?? "package.json"));
    if (target !== base && !target.startsWith(base + path.sep)) throw new Error("Dependency path escape.");
    void project;
    return readOne(rel(target), args.startLine, args.endLine);
  }
  if (tool === "search_dependency") return searchCode(String(args.query), `node_modules/${String(args.name)}`, args.maxResults ?? 100, !!args.fixedStrings, true);

  if (tool === "semantic_info") return semantic.info();
  if (tool === "workspace_symbols" || tool === "find_symbol") return { symbols: await semantic.workspaceSymbols(String(args.query ?? ""), args.limit ?? 200) };
  if (tool === "document_symbols") return { symbols: await semantic.documentSymbols(String(args.path), args.limit ?? 500) };
  if (tool === "find_definition") return { definitions: await semantic.definition({ path: args.path, line: args.line, column: args.column, name: args.name ?? args.query, limit: args.limit ?? 100 }) };
  if (tool === "find_references") return { references: await semantic.references({ path: args.path, line: args.line, column: args.column, name: args.name ?? args.query, limit: args.limit ?? 500 }) };
  if (tool === "find_implementations") return { implementations: await semantic.implementations({ path: String(args.path), line: Number(args.line), column: Number(args.column), limit: args.limit ?? 200 }) };
  if (tool === "get_hover") return semantic.hover({ path: String(args.path), line: Number(args.line), column: Number(args.column) });
  if (tool === "get_diagnostics") return { diagnostics: await semantic.diagnostics(args.path, args.limit ?? 500) };
  if (tool === "get_callers") return { callers: semantic.callers(String(args.name), args.limit ?? 300) };
  if (tool === "get_callees") return { callees: semantic.callees(String(args.name), args.limit ?? 300) };
  if (tool === "get_import_graph") return { edges: semantic.importGraph(args.limit ?? 2000) };

  if (tool === "snapshot_diagnostics") return verification.snapshotDiagnostics(args.paths ?? []);
  if (tool === "verify_changes") return verification.verify(args.paths ?? [], args.baselineId);
  if (tool === "apply_edits") { const result = await editing.applyEdits(args.files ?? []); semantic.invalidate(); context.invalidate(); return result; }
  if (tool === "format_changed_files") { const result = await editing.formatChangedFiles(args.paths ?? []); semantic.invalidate(); context.invalidate(); return result; }

  if (tool === "write_file") {
    const file = await safeWritePath(String(args.path));
    const exists = await fs.stat(file).then(() => true).catch(() => false);
    if (args.expectedHash && exists) {
      const actual = hashBuffer(await fs.readFile(file));
      if (actual !== args.expectedHash) throw new Error(`File changed since read. Expected ${args.expectedHash}, got ${actual}.`);
    }
    const content = String(args.content ?? "");
    await fs.writeFile(file, content, "utf8"); semantic.invalidate(); context.invalidate();
    return { ...(await fileMeta(file)), bytesWritten: Buffer.byteLength(content, "utf8") };
  }
  if (tool === "edit_file") {
    const file = await safeExistingPath(String(args.path));
    const original = await fs.readFile(file, "utf8");
    if (args.expectedHash) { const actual = hashBuffer(Buffer.from(original)); if (actual !== args.expectedHash) throw new Error(`File changed since read. Expected ${args.expectedHash}, got ${actual}.`); }
    const oldText = String(args.oldText); const count = original.split(oldText).length - 1;
    if (!count) throw new Error("oldText not found.");
    if (!args.replaceAll && count !== 1) throw new Error(`oldText occurs ${count} times.`);
    const updated = args.replaceAll ? original.split(oldText).join(String(args.newText ?? "")) : original.replace(oldText, String(args.newText ?? ""));
    await fs.writeFile(file, updated, "utf8"); semantic.invalidate(); context.invalidate();
    return { ...(await fileMeta(file)), replacements: args.replaceAll ? count : 1 };
  }
  if (tool === "apply_patch") {
    const patchText = String(args.patch ?? ""); validatePatchPaths(patchText);
    const check = await runDirect("git", ["apply", "--check", "--whitespace=nowarn", "-"], { input: patchText, timeoutMs: 20_000 });
    if (check.exitCode !== 0) throw new Error(`Patch check failed: ${check.stderr || check.stdout}`);
    const applied = await runDirect("git", ["apply", "--whitespace=nowarn", "-"], { input: patchText, timeoutMs: 20_000 });
    if (applied.exitCode !== 0) throw new Error(`Patch failed: ${applied.stderr || applied.stdout}`);
    semantic.invalidate(); context.invalidate(); return { applied: true };
  }

  if (tool === "git_status") { const r = await git(["status", "--short", "--branch"]); return { exitCode: r.exitCode, output: r.stdout + r.stderr }; }
  if (tool === "git_diff") { const argv = ["diff", "--no-ext-diff", "--unified=3"]; if (args.cached) argv.push("--cached"); if (args.path) argv.push("--", rel(await safeExistingPath(String(args.path)))); const r = await git(argv); return { exitCode: r.exitCode, diff: r.stdout + r.stderr }; }
  if (tool === "git_log") { const r = await git(["log", `-${Math.min(args.limit ?? 20, 100)}`, "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s", ...(args.path ? ["--", String(args.path)] : [])]); return { output: r.stdout }; }
  if (tool === "git_show") { const r = await git(["show", "--stat", "--oneline", "--decorate", String(args.ref ?? "HEAD")]); return { output: r.stdout + r.stderr }; }
  if (tool === "git_blame") { const p = String(args.path); if (isSensitivePath(p)) throw new Error(`Access blocked by sensitive-path policy: ${p}`); const r = await git(["blame", "--line-porcelain", ...(args.startLine && args.endLine ? ["-L", `${args.startLine},${args.endLine}`] : []), "--", p]); return { output: r.stdout }; }
  if (tool === "git_file_history") { const p = String(args.path); if (isSensitivePath(p)) throw new Error(`Access blocked by sensitive-path policy: ${p}`); const r = await git(["log", "--follow", `-${Math.min(args.limit ?? 30, 100)}`, "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s", "--", p]); return { output: r.stdout }; }
  if (tool === "git_stage") { const paths = (args.paths ?? []).map(String); return gitWrite("add", paths.join(" "), ["add", "--", ...paths], request.requestId, args.approvalToken); }
  if (tool === "git_unstage") { const paths = (args.paths ?? []).map(String); return gitWrite("restore --staged", paths.join(" "), ["restore", "--staged", "--", ...paths], request.requestId, args.approvalToken); }
  if (tool === "git_commit") {
    const staged = await git(["diff", "--cached", "--name-only"]); const stagedPaths = staged.stdout.split("\n").filter(Boolean);
    if (!stagedPaths.length) throw new Error("No staged changes to commit.");
    if (Array.isArray(args.expectedPaths) && stagedPaths.some((p) => !args.expectedPaths.includes(p))) throw new Error(`Unexpected staged changes: ${stagedPaths.filter((p) => !args.expectedPaths.includes(p)).join(", ")}`);
    return gitWrite("commit", String(args.message), ["commit", "-m", String(args.message)], request.requestId, args.approvalToken);
  }
  if (tool === "git_push") {
    if (args.force) throw new Error("Force push is blocked by default.");
    const argv = ["push"]; if (args.remote) argv.push(String(args.remote)); if (args.branch) argv.push(String(args.branch));
    return gitWrite("push", argv.slice(1).join(" "), argv, request.requestId, args.approvalToken);
  }

  if (tool === "sandbox_info") return sandbox.info();
  if (tool === "sandbox_smoke_test") return sandbox.smokeTest();
  if (tool === "terminal_preflight") return terminalPreflight(String(args.command ?? ""), String(args.cwd ?? "."), request.requestId);
  if (tool === "terminal_history") return terminalHistory.query({ workspaceKey: WORKSPACE_KEY, query: String(args.query ?? ""), limit: args.limit ?? 50, event: args.event ?? "started" });
  if (tool === "run_command") {
    const started = await runGuardedCommand(String(args.command), { cwd: args.cwd ?? ".", timeoutMs: args.timeoutMs ?? 0, requestId: request.requestId, ownerSessionId: request.sessionId, approvalToken: args.approvalToken });
    if (!("processId" in started)) return started;
    await new Promise((resolve) => setTimeout(resolve, Math.min(Math.max(args.yieldMs ?? 1000, 0), 10_000)));
    return processManager.snapshot(started.processId);
  }
  if (tool === "exec_start") return runGuardedCommand(String(args.command), { cwd: args.cwd ?? ".", timeoutMs: args.timeoutMs ?? 0, requestId: request.requestId, ownerSessionId: request.sessionId, usePty: false, approvalToken: args.approvalToken });
  if (tool === "pty_start") return runGuardedCommand(String(args.command), { cwd: args.cwd ?? ".", timeoutMs: args.timeoutMs ?? 0, requestId: request.requestId, ownerSessionId: request.sessionId, usePty: true, approvalToken: args.approvalToken });
  if (tool === "exec_poll" || tool === "pty_poll") return processManager.snapshot(String(args.processId), { stdout: args.stdoutCursor, stderr: args.stderrCursor });
  if (tool === "process_poll") return processManager.snapshot(String(args.processId), { stdout: args.cursor, stderr: args.cursor });
  if (tool === "exec_write" || tool === "pty_write" || tool === "process_write") return processManager.write(String(args.processId), String(args.input ?? ""));
  if (tool === "pty_resize") return processManager.resize(String(args.processId), Number(args.cols), Number(args.rows));
  if (tool === "exec_signal" || tool === "pty_signal") return processManager.signal(String(args.processId), (args.signal ?? "SIGTERM") as NodeJS.Signals);
  if (tool === "exec_cancel") return processManager.cancel(String(args.processId), String(args.reason ?? "cancelled"));
  if (tool === "exec_kill" || tool === "pty_kill" || tool === "process_kill") return processManager.signal(String(args.processId), (args.signal ?? "SIGTERM") as NodeJS.Signals);
  if (tool === "process_list") return { processes: processManager.list() };
  throw new Error(`Unknown tool: ${tool}`);
}

const watcher = chokidar.watch(root, {
  ignored: (p) => {
    const relative = rel(p);
    return isSensitivePath(relative) || /(^|\/)(\.git|node_modules|\.next|dist|build|target|\.venv|venv)(\/|$)/.test(relative);
  },
  ignoreInitial: true,
  persistent: true,
});
watcher.on("all", (event, changed) => {
  const relative = rel(changed);
  semantic.invalidate(); context.invalidate(); metadataEpoch++;
  if (relative === ".gitignore") void reloadIgnore();
  log("debug", "workspace.changed", { event, path: relative, metadataEpoch });
});

async function clientCapabilities() {
  const semanticInfo = await semantic.info();
  const sandboxInfo = await sandbox.info();
  let pty = false;
  try { const dynamicImport = new Function("m", "return import(m)") as (m: string) => Promise<any>; await dynamicImport("node-pty"); pty = true; } catch {}
  return {
    filesystem: true,
    git: true,
    shell: ALLOW_SHELL,
    pty,
    sandbox: sandboxInfo.mode,
    semanticProviders: ["typescript", ...semanticInfo.providers.filter((p) => p.installed).map((p) => p.id)],
    idempotency: true,
    cancellation: true,
    approvals: true,
    terminalChatApproval: true,
    terminalHistory: true,
    mcpHub: true,
  };
}

async function executeRequest(message: ToolCallMessage | any) {
  const requestId = String(message.requestId ?? message.id ?? "");
  const tool = String(message.tool ?? "");
  const idempotencyKey = typeof message.idempotencyKey === "string" ? message.idempotencyKey : undefined;
  if (isSideEffectingTool(tool) && idempotencyKey) {
    const existing = await journal.get(idempotencyKey);
    if (existing?.status === "completed") return existing.result;
    if (existing?.status === "started") throw new Error(`Duplicate request is already/was ambiguously started: ${idempotencyKey}`);
    await journal.start(idempotencyKey, tool);
    try {
      const result = await handleTool(tool, message.args ?? {}, { requestId, sessionId: message.sessionId });
      await journal.complete(idempotencyKey, result);
      return result;
    } catch (error) {
      await journal.fail(idempotencyKey, error instanceof Error ? error.message : String(error));
      throw error;
    }
  }
  return handleTool(tool, message.args ?? {}, { requestId, sessionId: message.sessionId });
}

async function connect() {
  log("info", "client.connecting", { server: SERVER_URL, protocolVersion: PROTOCOL_VERSION, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, projectRoot: root });
  const ws = new WebSocket(SERVER_URL!);
  activeSocket = ws;
  ws.on("open", async () => {
    const capabilities = await clientCapabilities();
    ws.send(JSON.stringify({
      type: "register",
      protocolVersion: PROTOCOL_VERSION,
      token: localCredential ? undefined : LEGACY_DEVICE_TOKEN,
      credentialId: localCredential?.credentialId,
      credentialSecret: localCredential?.credentialSecret,
      deviceId: DEVICE_ID,
      deviceName: DEVICE_NAME,
      workspaceId: WORKSPACE_ID,
      workspaceName: WORKSPACE_NAME,
      projectRoot: root,
      capabilities,
    }));
  });
  ws.on("message", async (raw) => {
    let message: any;
    try { message = JSON.parse(raw.toString()); } catch { return; }
    if (message.type === "registered") {
      reconnectDelay = 1000;
      log("info", "client.registered", { protocolVersion: message.protocolVersion, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, shell: ALLOW_SHELL, approvalMode: APPROVAL_MODE });
      return;
    }
    if (message.type === "ping") {
      ws.send(JSON.stringify({ type: "pong", protocolVersion: PROTOCOL_VERSION, ts: Date.now(), deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID }));
      return;
    }
    if (message.type === "tool_cancel") {
      const requestId = String(message.requestId ?? "");
      const result = processManager.cancelRequest(requestId, String(message.reason ?? "tool cancelled"));
      await audit({ event: "tool.cancel", requestId, workspaceKey: WORKSPACE_KEY, status: result.cancelled ? "cancelled" : "not-found" });
      return;
    }
    if (message.type !== "tool_call") return;
    const requestId = String(message.requestId ?? message.id ?? "");
    const startedAt = Date.now();
    log("info", "tool.received", { requestId, tool: message.tool, args: summarizeToolArgs(message.tool, message.args) });
    await audit({ event: "tool.received", requestId, workspaceKey: WORKSPACE_KEY, tool: message.tool });
    try {
      const result = await executeRequest(message);
      ws.send(JSON.stringify({ type: "tool_result", protocolVersion: PROTOCOL_VERSION, requestId, id: requestId, ok: true, result, metadata: { durationMs: Date.now() - startedAt } }));
      log("info", "tool.completed", { requestId, tool: message.tool, durationMs: Date.now() - startedAt });
      await audit({ event: "tool.completed", requestId, workspaceKey: WORKSPACE_KEY, tool: message.tool, status: "ok" });
    } catch (error) {
      const normalized = normalizeError(error);
      ws.send(JSON.stringify({ type: "tool_result", protocolVersion: PROTOCOL_VERSION, requestId, id: requestId, ok: false, ...normalized, error: normalized.errorMessage, metadata: { durationMs: Date.now() - startedAt } }));
      log("error", "tool.failed", { requestId, tool: message.tool, durationMs: Date.now() - startedAt, error });
      await audit({ event: "tool.failed", requestId, workspaceKey: WORKSPACE_KEY, tool: message.tool, status: normalized.errorCode, detail: normalized.errorMessage });
    }
  });
  ws.on("close", (code, reason) => {
    log("warn", "client.disconnected", { code, reason: reason.toString(), reconnectInMs: reconnectDelay });
    if (activeSocket === ws) activeSocket = null;
    const jitter = Math.floor(Math.random() * Math.min(1000, reconnectDelay / 2));
    setTimeout(() => void connect(), reconnectDelay + jitter);
    reconnectDelay = Math.min(reconnectDelay * 2, 30_000);
  });
  ws.on("error", (error) => log("error", "client.socket_error", { error }));
}

async function shutdown() {
  await Promise.allSettled([semantic.shutdown(), mcpHub.shutdown(), watcher.close()]);
  activeSocket?.close();
}

process.on("SIGINT", async () => { await shutdown(); process.exit(0); });
process.on("SIGTERM", async () => { await shutdown(); process.exit(0); });

log("info", "client.started", { version: "1.5.0-dev.0", protocolVersion: PROTOCOL_VERSION, deviceId: DEVICE_ID, workspaceId: WORKSPACE_ID, projectRoot: root, shell: ALLOW_SHELL, approvalMode: APPROVAL_MODE, terminalApproval: "chat-mediated", networkPolicy: NETWORK_POLICY, hostname: os.hostname() });
void connect();
