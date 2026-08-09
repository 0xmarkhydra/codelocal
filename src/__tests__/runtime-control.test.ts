import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { acquireRuntimeLease, runtimeSummary, sendRuntimeCommand, startRuntimeControlServer } from "../runtime-control.js";

test("runtime lease enforces one CodeLocal daemon per state directory", async () => {
  const stateDir = await mkdtemp(path.join(os.tmpdir(), "codelocal-runtime-"));
  try {
    const first = await acquireRuntimeLease("1.0.0", stateDir);
    assert.equal(first.acquired, true);

    const second = await acquireRuntimeLease("1.0.0", stateDir);
    assert.equal(second.acquired, false);
    assert.equal(second.record.pid, process.pid);

    await first.release();
    const third = await acquireRuntimeLease("1.0.1", stateDir);
    assert.equal(third.acquired, true);
    await third.release();
  } finally {
    await rm(stateDir, { recursive: true, force: true });
  }
});

test("runtime control socket supports status and hot reload", async () => {
  const stateDir = await mkdtemp(path.join(os.tmpdir(), "codelocal-runtime-ipc-"));
  const lease = await acquireRuntimeLease("1.0.0", stateDir);
  let reloads = 0;
  const control = await startRuntimeControlServer(async (command) => {
    if (command.type === "status") return { authorizedWorkspaces: [{ workspaceId: "a" }], activeWorkspaces: [] };
    if (command.type === "reload") return { reloaded: true, count: ++reloads };
    return { stopping: true };
  }, stateDir);

  try {
    const status = await sendRuntimeCommand({ type: "status" }, stateDir) as { authorizedWorkspaces?: unknown[] } | null;
    assert.equal(status?.authorizedWorkspaces?.length, 1);

    const reload = await sendRuntimeCommand({ type: "reload" }, stateDir) as { reloaded?: boolean; count?: number } | null;
    assert.equal(reload?.reloaded, true);
    assert.equal(reload?.count, 1);

    const summary = await runtimeSummary(stateDir);
    assert.equal(summary.running, true);
    assert.equal(summary.responsive, true);
  } finally {
    await control.close();
    await lease.release();
    await rm(stateDir, { recursive: true, force: true });
  }
});
