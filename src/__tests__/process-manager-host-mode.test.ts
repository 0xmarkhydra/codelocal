import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { ProcessManager } from "../process-manager.js";

test("process manager executes through the host-policy lane", async () => {
  const root = process.cwd();
  const manager = new ProcessManager(root, "test-workspace");
  const command = `${JSON.stringify(process.execPath)} --version`;
  const started = await manager.start(command, { cwd: root, timeoutMs: 10_000 });
  assert.equal(started.executionMode, "host-policy");
  assert.equal(started.cwd, ".");

  for (let i = 0; i < 100; i++) {
    const current = manager.snapshot(started.processId);
    if (!current.running) {
      assert.equal(current.exitCode, 0);
      assert.match(current.stdout.text, /^v\d+/);
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  manager.cancel(started.processId, "test timeout");
  assert.fail("host-policy process did not finish in time");
});

test("process output cursors use UTF-8 byte offsets", async () => {
  const root = process.cwd();
  const manager = new ProcessManager(root, "test-workspace");
  const script = "process.stdout.write('😀é')";
  const started = await manager.start(`${JSON.stringify(process.execPath)} -e ${JSON.stringify(script)}`, { cwd: root, timeoutMs: 10_000 });

  for (let i = 0; i < 100; i++) {
    const current = manager.snapshot(started.processId);
    if (!current.running) {
      assert.equal(current.stdout.text, "😀é");
      assert.equal(current.stdout.cursor, Buffer.byteLength("😀é", "utf8"));
      assert.equal(manager.snapshot(started.processId, { stdout: Buffer.byteLength("😀", "utf8") }).stdout.text, "é");
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }

  manager.cancel(started.processId, "test timeout");
  assert.fail("unicode output process did not finish in time");
});

test("finished processes release request cancellation mappings", async () => {
  const root = process.cwd();
  const manager = new ProcessManager(root, "test-workspace");
  const requestId = "request-cleanup-test";
  const started = await manager.start(`${JSON.stringify(process.execPath)} --version`, { cwd: root, timeoutMs: 10_000, requestId });

  for (let i = 0; i < 100; i++) {
    const current = manager.snapshot(started.processId);
    if (!current.running) {
      const cancelled = manager.cancelRequest(requestId);
      assert.deepEqual(cancelled, { cancelled: false, reason: "no process associated with request" });
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }

  manager.cancel(started.processId, "test timeout");
  assert.fail("process did not finish in time");
});
