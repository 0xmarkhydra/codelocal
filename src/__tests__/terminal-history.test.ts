import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, rm, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { TerminalHistory } from "../terminal-history.js";
import type { ProcessRecord } from "../process-manager.js";

test("terminal history records executed commands locally without leaking secrets", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "codelocal-terminal-history-"));
  try {
    const file = path.join(temp, "history.jsonl");
    const history = new TerminalHistory(file);
    const startedAt = Date.now() - 25;
    await history.started({
      workspaceKey: "device::workspace",
      processId: "p1",
      requestId: "r1",
      sessionId: "s1",
      cwd: ".",
      command: "GITHUB_TOKEN=super-secret npm test",
      riskLevel: "SAFE",
      matchedRules: [],
      approval: "automatic",
      startedAt,
      executionMode: "host-policy",
    });

    const record: ProcessRecord = {
      processId: "p1",
      workspaceKey: "device::workspace",
      ownerSessionId: "s1",
      pid: 123,
      command: "GITHUB_TOKEN=super-secret npm test",
      cwd: temp,
      startedAt,
      lastActivityAt: Date.now(),
      status: "exited",
      exitCode: 0,
      signal: null,
      stdout: { data: Buffer.alloc(0), baseOffset: 0, totalBytes: 0 },
      stderr: { data: Buffer.alloc(0), baseOffset: 0, totalBytes: 0 },
      timeoutAt: null,
      pty: false,
      executionMode: "host-policy",
    };
    await history.finished(record);

    const raw = await readFile(file, "utf8");
    assert.doesNotMatch(raw, /super-secret/);
    assert.match(raw, /GITHUB_TOKEN=\[REDACTED\]/);

    const starts = await history.query({ workspaceKey: "device::workspace", query: "npm test", event: "started" });
    assert.equal(starts.records.length, 1);
    assert.equal(starts.records[0].approval, "automatic");

    const finished = await history.query({ workspaceKey: "device::workspace", event: "finished" });
    assert.equal(finished.records.length, 1);
    assert.equal(finished.records[0].exitCode, 0);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});
