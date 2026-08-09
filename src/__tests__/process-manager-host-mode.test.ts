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
