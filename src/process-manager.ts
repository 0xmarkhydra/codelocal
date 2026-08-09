import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { randomUUID } from "node:crypto";
import path from "node:path";

const MAX_BUFFER_BYTES = Number(process.env.CODELOCAL_MAX_PROCESS_BUFFER_BYTES ?? 2 * 1024 * 1024);
const MAX_PROCESSES = Number(process.env.CODELOCAL_MAX_PROCESSES ?? 64);

type Status = "running" | "exited" | "cancelled" | "failed";
type ExecutionMode = "host-policy";

type StreamBuffer = { text: string; baseOffset: number; totalBytes: number };

type PtyLike = {
  pid: number;
  write(data: string): void;
  resize(cols: number, rows: number): void;
  kill(signal?: string): void;
  onData(cb: (data: string) => void): { dispose?: () => void } | void;
  onExit(cb: (event: { exitCode: number; signal?: number }) => void): { dispose?: () => void } | void;
};

export type ProcessRecord = {
  processId: string;
  workspaceKey: string;
  ownerSessionId?: string;
  pid: number | null;
  command: string;
  cwd: string;
  startedAt: number;
  lastActivityAt: number;
  status: Status;
  exitCode: number | null;
  signal: string | null;
  stdout: StreamBuffer;
  stderr: StreamBuffer;
  timeoutAt: number | null;
  pty: boolean;
  executionMode: ExecutionMode;
  child?: ChildProcessWithoutNullStreams;
  terminal?: PtyLike;
};

function append(buffer: StreamBuffer, value: string) {
  const bytes = Buffer.byteLength(value, "utf8");
  buffer.totalBytes += bytes;
  buffer.text += value;
  const currentBytes = Buffer.byteLength(buffer.text, "utf8");
  if (currentBytes > MAX_BUFFER_BYTES) {
    const keep = buffer.text.slice(-MAX_BUFFER_BYTES);
    const keptBytes = Buffer.byteLength(keep, "utf8");
    buffer.baseOffset += currentBytes - keptBytes;
    buffer.text = keep;
  }
}

function readBuffer(buffer: StreamBuffer, cursor?: number) {
  const requested = Math.max(cursor ?? buffer.baseOffset, buffer.baseOffset);
  const relative = Math.max(0, requested - buffer.baseOffset);
  return {
    text: buffer.text.slice(relative),
    cursor: buffer.totalBytes,
    truncatedBeforeCursor: (cursor ?? buffer.baseOffset) < buffer.baseOffset,
  };
}

async function loadNodePty(): Promise<any | null> {
  try {
    const dynamicImport = new Function("m", "return import(m)") as (m: string) => Promise<any>;
    return await dynamicImport("node-pty");
  } catch {
    return null;
  }
}

function hostShell(command: string, cwd: string) {
  const shell = process.env.SHELL || (process.platform === "win32" ? "cmd.exe" : "/bin/zsh");
  // Use the runtime's inherited environment without starting a login shell. This preserves
  // PATH/credential helpers while avoiding arbitrary profile startup hooks on every command.
  const args = process.platform === "win32" ? ["/d", "/s", "/c", command] : ["-c", command];
  return { command: shell, args, cwd };
}

export class ProcessManager {
  private records = new Map<string, ProcessRecord>();
  private requestToProcess = new Map<string, string>();
  private settledNotified = new Set<string>();

  constructor(
    private workspaceRoot: string,
    private workspaceKey: string,
    private onOutput?: (record: ProcessRecord, stream: "stdout" | "stderr", text: string) => void,
    private onSettled?: (record: ProcessRecord) => void | Promise<void>,
  ) {}

  private prune() {
    const finished = [...this.records.values()].filter((x) => x.status !== "running").sort((a, b) => a.startedAt - b.startedAt);
    while (this.records.size >= MAX_PROCESSES && finished.length) {
      const victim = finished.shift()!;
      this.records.delete(victim.processId);
      this.settledNotified.delete(victim.processId);
    }
    if (this.records.size >= MAX_PROCESSES) throw new Error(`Too many active CodeLocal processes (${MAX_PROCESSES}).`);
  }

  private baseRecord(command: string, cwd: string, executionMode: ExecutionMode, ownerSessionId?: string): ProcessRecord {
    return {
      processId: randomUUID(),
      workspaceKey: this.workspaceKey,
      ownerSessionId,
      pid: null,
      command,
      cwd,
      startedAt: Date.now(),
      lastActivityAt: Date.now(),
      status: "running",
      exitCode: null,
      signal: null,
      stdout: { text: "", baseOffset: 0, totalBytes: 0 },
      stderr: { text: "", baseOffset: 0, totalBytes: 0 },
      timeoutAt: null,
      pty: false,
      executionMode,
    };
  }

  private emit(record: ProcessRecord, stream: "stdout" | "stderr", text: string) {
    record.lastActivityAt = Date.now();
    append(stream === "stdout" ? record.stdout : record.stderr, text);
    this.onOutput?.(record, stream, text);
  }

  private notifySettled(record: ProcessRecord) {
    if (this.settledNotified.has(record.processId)) return;
    this.settledNotified.add(record.processId);
    Promise.resolve(this.onSettled?.(record)).catch(() => undefined);
  }

  async start(command: string, options: { cwd: string; timeoutMs?: number; ownerSessionId?: string; requestId?: string; usePty?: boolean; cols?: number; rows?: number }) {
    this.prune();
    const executionMode: ExecutionMode = "host-policy";
    const record = this.baseRecord(command, options.cwd, executionMode, options.ownerSessionId);
    this.records.set(record.processId, record);
    if (options.requestId) this.requestToProcess.set(options.requestId, record.processId);

    const wrap = async () => hostShell(command, options.cwd);

    if (options.usePty) {
      const nodePty = await loadNodePty();
      if (nodePty) {
        const wrapped = await wrap();
        const terminal = nodePty.spawn(wrapped.command, wrapped.args, {
          name: process.env.TERM || "xterm-256color",
          cols: options.cols ?? 120,
          rows: options.rows ?? 36,
          cwd: wrapped.cwd,
          env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CODELOCAL_EXECUTION_MODE: executionMode },
        }) as PtyLike;
        record.pty = true;
        record.terminal = terminal;
        record.pid = terminal.pid;
        terminal.onData((data) => this.emit(record, "stdout", data));
        terminal.onExit(({ exitCode, signal }) => {
          record.status = record.status === "cancelled" ? "cancelled" : "exited";
          record.exitCode = exitCode;
          record.signal = signal == null ? null : String(signal);
          record.lastActivityAt = Date.now();
          this.notifySettled(record);
        });
        this.scheduleTimeout(record, options.timeoutMs);
        return this.snapshot(record.processId);
      }
    }

    const wrapped = await wrap();
    const child = spawn(wrapped.command, wrapped.args, {
      cwd: wrapped.cwd,
      env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat", CI: process.env.CI ?? "1", CODELOCAL_EXECUTION_MODE: executionMode },
      stdio: "pipe",
    }) as ChildProcessWithoutNullStreams;
    record.child = child;
    record.pid = child.pid ?? null;
    child.stdout.on("data", (d) => this.emit(record, "stdout", d.toString()));
    child.stderr.on("data", (d) => this.emit(record, "stderr", d.toString()));
    child.on("error", (error) => {
      record.status = "failed";
      record.exitCode = -1;
      this.emit(record, "stderr", `\n[process error] ${error.message}\n`);
    });
    child.on("close", (code, signal) => {
      if (record.status !== "cancelled" && record.status !== "failed") record.status = "exited";
      record.exitCode = code;
      record.signal = signal ?? null;
      record.lastActivityAt = Date.now();
      this.notifySettled(record);
    });
    this.scheduleTimeout(record, options.timeoutMs);
    return this.snapshot(record.processId);
  }

  private scheduleTimeout(record: ProcessRecord, timeoutMs?: number) {
    if (!timeoutMs || timeoutMs <= 0) return;
    record.timeoutAt = Date.now() + timeoutMs;
    setTimeout(() => {
      const current = this.records.get(record.processId);
      if (current?.status === "running") this.cancel(record.processId, "timeout");
    }, timeoutMs).unref?.();
  }

  snapshot(processId: string, cursors: { stdout?: number; stderr?: number } = {}) {
    const record = this.records.get(processId);
    if (!record) throw new Error("Unknown processId.");
    return {
      processId: record.processId,
      workspaceKey: record.workspaceKey,
      ownerSessionId: record.ownerSessionId ?? null,
      pid: record.pid,
      command: record.command,
      cwd: path.relative(this.workspaceRoot, record.cwd).split(path.sep).join("/") || ".",
      startedAt: record.startedAt,
      lastActivityAt: record.lastActivityAt,
      status: record.status,
      running: record.status === "running",
      exitCode: record.exitCode,
      signal: record.signal,
      timeoutAt: record.timeoutAt,
      pty: record.pty,
      executionMode: record.executionMode,
      stdout: readBuffer(record.stdout, cursors.stdout),
      stderr: readBuffer(record.stderr, cursors.stderr),
    };
  }

  list() {
    return [...this.records.values()].map((record) => ({
      processId: record.processId,
      pid: record.pid,
      command: record.command,
      cwd: path.relative(this.workspaceRoot, record.cwd).split(path.sep).join("/") || ".",
      status: record.status,
      exitCode: record.exitCode,
      signal: record.signal,
      startedAt: record.startedAt,
      lastActivityAt: record.lastActivityAt,
      pty: record.pty,
      executionMode: record.executionMode,
    }));
  }

  write(processId: string, input: string) {
    const record = this.records.get(processId);
    if (!record || record.status !== "running") throw new Error("Process not running.");
    if (record.terminal) record.terminal.write(input);
    else if (record.child) record.child.stdin.write(input);
    else throw new Error("Process stdin unavailable.");
    record.lastActivityAt = Date.now();
    return { written: Buffer.byteLength(input, "utf8") };
  }

  resize(processId: string, cols: number, rows: number) {
    const record = this.records.get(processId);
    if (!record?.terminal) throw new Error("PTY resize is not available for this process.");
    record.terminal.resize(cols, rows);
    return { resized: true, cols, rows };
  }

  signal(processId: string, signal: NodeJS.Signals = "SIGTERM") {
    const record = this.records.get(processId);
    if (!record) throw new Error("Unknown processId.");
    if (record.status !== "running") return { signalled: false, status: record.status };
    if (record.terminal) record.terminal.kill(signal);
    else record.child?.kill(signal);
    record.signal = signal;
    record.lastActivityAt = Date.now();
    return { signalled: true, signal };
  }

  cancel(processId: string, reason = "cancelled") {
    const record = this.records.get(processId);
    if (!record) throw new Error("Unknown processId.");
    if (record.status === "running") {
      record.status = "cancelled";
      this.emit(record, "stderr", `\n[CodeLocal] ${reason}\n`);
      if (record.terminal) record.terminal.kill("SIGTERM");
      else record.child?.kill("SIGTERM");
    }
    return { cancelled: true, processId };
  }

  cancelRequest(requestId: string, reason = "tool request cancelled") {
    const processId = this.requestToProcess.get(requestId);
    if (!processId) return { cancelled: false, reason: "no process associated with request" };
    return this.cancel(processId, reason);
  }
}
