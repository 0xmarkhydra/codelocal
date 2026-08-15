import net from "node:net";
import os from "node:os";
import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import { DEFAULT_STATE_DIR, ensurePrivateDir, readJsonFile } from "./state.js";

export type RuntimeLeaseRecord = {
  pid: number;
  instanceId: string;
  startedAt: number;
  version: string;
  endpoint: string;
};

export type RuntimeControlCommand =
  | { type: "status" }
  | { type: "reload" }
  | { type: "shutdown" };

export type RuntimeLease = {
  acquired: boolean;
  record: RuntimeLeaseRecord;
  release: () => Promise<void>;
};

function runtimeLockPath(stateDir = DEFAULT_STATE_DIR) {
  return path.join(stateDir, "runtime.lock");
}

export function runtimeEndpoint(stateDir = DEFAULT_STATE_DIR) {
  if (process.platform === "win32") {
    const digest = createHash("sha256").update(path.resolve(stateDir)).digest("hex").slice(0, 20);
    return `\\\\.\\pipe\\codelocal-${digest}`;
  }
  return path.join(stateDir, "runtime.sock");
}

function processAlive(pid: number) {
  if (!Number.isInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return (error as NodeJS.ErrnoException)?.code === "EPERM";
  }
}

async function delay(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function endpointExists(endpoint: string) {
  if (process.platform === "win32") return true;
  return fs.stat(endpoint).then(() => true).catch(() => false);
}

async function sendRuntimeCommandToRecord(
  record: RuntimeLeaseRecord,
  command: RuntimeControlCommand,
  timeoutMs = 1_200,
): Promise<unknown | null> {
  return new Promise((resolve) => {
    const socket = net.createConnection(record.endpoint);
    let settled = false;
    let buffer = "";
    const finish = (value: unknown | null) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      socket.destroy();
      resolve(value);
    };
    const timer = setTimeout(() => finish(null), Math.max(100, timeoutMs));
    timer.unref?.();

    socket.setEncoding("utf8");
    socket.once("connect", () => socket.write(`${JSON.stringify(command)}\n`));
    socket.on("data", (chunk) => {
      buffer += chunk;
      const newline = buffer.indexOf("\n");
      if (newline < 0) return;
      try {
        const response = JSON.parse(buffer.slice(0, newline)) as { ok?: boolean; instanceId?: string; result?: unknown };
        if (!response.ok || response.instanceId !== record.instanceId) { finish(null); return; }
        finish(response.result ?? true);
      } catch {
        finish(null);
      }
    });
    socket.once("error", () => finish(null));
    socket.once("close", () => { if (!settled) finish(null); });
  });
}

export async function readRuntimeLease(stateDir = DEFAULT_STATE_DIR) {
  const record = await readJsonFile<RuntimeLeaseRecord | null>(runtimeLockPath(stateDir), null);
  if (!record || !processAlive(record.pid)) return null;
  return record;
}

export async function acquireRuntimeLease(version: string, stateDir = DEFAULT_STATE_DIR): Promise<RuntimeLease> {
  await ensurePrivateDir(stateDir);
  const lockFile = runtimeLockPath(stateDir);
  const endpoint = runtimeEndpoint(stateDir);
  const deadline = Date.now() + 2_000;

  while (Date.now() < deadline) {
    const instanceId = randomUUID();
    const record: RuntimeLeaseRecord = { pid: process.pid, instanceId, startedAt: Date.now(), version, endpoint };
    try {
      const handle = await fs.open(lockFile, "wx", 0o600);
      try {
        await handle.writeFile(`${JSON.stringify(record)}\n`, "utf8");
        await handle.sync();
      } finally {
        await handle.close();
      }
      if (process.platform !== "win32") await fs.chmod(lockFile, 0o600).catch(() => undefined);
      return {
        acquired: true,
        record,
        release: async () => {
          const current = await readJsonFile<RuntimeLeaseRecord | null>(lockFile, null);
          if (current?.instanceId === instanceId) await fs.unlink(lockFile).catch(() => undefined);
        },
      };
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code !== "EEXIST") throw error;
      const existing = await readJsonFile<RuntimeLeaseRecord | null>(lockFile, null);
      if (existing && processAlive(existing.pid)) {
        const live = await sendRuntimeCommandToRecord(existing, { type: "status" }, 350);
        if (live != null) return { acquired: false, record: existing, release: async () => undefined };

        const ageMs = Date.now() - existing.startedAt;
        if (ageMs < 1_500) {
          await delay(50);
          continue;
        }

        if (!(await endpointExists(existing.endpoint || endpoint))) {
          await fs.unlink(lockFile).catch(() => undefined);
          continue;
        }

        throw new Error(`CodeLocal runtime PID ${existing.pid} is alive but its control channel is unresponsive. Refusing to start a duplicate runtime.`);
      }
      if (!existing) {
        const stat = await fs.stat(lockFile).catch(() => null);
        if (stat && Date.now() - stat.mtimeMs < 500) {
          await delay(25);
          continue;
        }
      }
      await fs.unlink(lockFile).catch(() => undefined);
      if (process.platform !== "win32") await fs.unlink(endpoint).catch(() => undefined);
    }
  }

  throw new Error("Unable to acquire the CodeLocal runtime lock.");
}

export async function startRuntimeControlServer(
  handler: (command: RuntimeControlCommand) => unknown | Promise<unknown>,
  stateDir: string,
  instanceId: string,
) {
  await ensurePrivateDir(stateDir);
  const endpoint = runtimeEndpoint(stateDir);
  if (process.platform !== "win32") await fs.unlink(endpoint).catch(() => undefined);

  const server = net.createServer((socket) => {
    socket.setEncoding("utf8");
    let buffer = "";
    let handled = false;

    const respond = (payload: unknown) => {
      if (socket.destroyed) return;
      const envelope = payload && typeof payload === "object"
        ? { ...(payload as Record<string, unknown>), instanceId }
        : { ok: false, error: "invalid_runtime_control_response", instanceId };
      socket.end(`${JSON.stringify(envelope)}\n`);
    };

    socket.on("data", (chunk) => {
      if (handled) return;
      buffer += chunk;
      if (Buffer.byteLength(buffer, "utf8") > 64 * 1024) {
        handled = true;
        respond({ ok: false, error: "runtime_control_request_too_large" });
        return;
      }
      const newline = buffer.indexOf("\n");
      if (newline < 0) return;
      handled = true;
      void (async () => {
        try {
          const command = JSON.parse(buffer.slice(0, newline)) as RuntimeControlCommand;
          if (!command || !["status", "reload", "shutdown"].includes(command.type)) throw new Error("invalid_runtime_control_command");
          const result = await handler(command);
          respond({ ok: true, result });
        } catch (error) {
          respond({ ok: false, error: error instanceof Error ? error.message : String(error) });
        }
      })();
    });
  });

  await new Promise<void>((resolve, reject) => {
    const onError = (error: Error) => { server.off("listening", onListening); reject(error); };
    const onListening = () => { server.off("error", onError); resolve(); };
    server.once("error", onError);
    server.once("listening", onListening);
    server.listen(endpoint);
  });

  if (process.platform !== "win32") await fs.chmod(endpoint, 0o600).catch(() => undefined);

  let closed = false;
  return {
    endpoint,
    close: async () => {
      if (closed) return;
      closed = true;
      await new Promise<void>((resolve) => server.close(() => resolve())).catch(() => undefined);
      if (process.platform !== "win32") await fs.unlink(endpoint).catch(() => undefined);
    },
  };
}

export async function sendRuntimeCommand(
  command: RuntimeControlCommand,
  stateDir = DEFAULT_STATE_DIR,
  timeoutMs = 1_200,
): Promise<unknown | null> {
  const lease = await readRuntimeLease(stateDir);
  if (!lease) return null;
  return sendRuntimeCommandToRecord(lease, command, timeoutMs);
}

export async function runtimeSummary(stateDir = DEFAULT_STATE_DIR) {
  const lease = await readRuntimeLease(stateDir);
  if (!lease) return { running: false as const };
  const live = await sendRuntimeCommand({ type: "status" }, stateDir).catch(() => null);
  return {
    running: true as const,
    pid: lease.pid,
    version: lease.version,
    startedAt: lease.startedAt,
    responsive: live != null,
    ...(live && typeof live === "object" ? { detail: live } : {}),
  };
}

export function runtimeStateDir() {
  return process.env.CODELOCAL_STATE_DIR ?? path.join(os.homedir(), ".codelocal");
}
