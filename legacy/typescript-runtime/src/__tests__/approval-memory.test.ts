import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { promises as fs } from "node:fs";
import { ApprovalMemory } from "../approval-memory.js";
import type { PolicyDecision } from "../security-policy.js";

const rememberable: PolicyDecision = {
  riskLevel: "REVIEW",
  matchedRules: ["Git write action"],
  requiresApproval: true,
  blocked: false,
  redactedCommand: "git push origin dev",
  reason: "Git write action",
  approvalPolicy: "rememberable",
  approvalKey: "git.push:origin:dev:remote-demo",
  approvalLabel: "Git push origin dev",
};

function critical(): PolicyDecision {
  return {
    ...rememberable,
    riskLevel: "CRITICAL",
    approvalPolicy: "always",
    approvalKey: undefined,
    approvalLabel: undefined,
    redactedCommand: "git push --force origin dev",
    reason: "destructive Git action",
  };
}

test("remembered approvals are local, workspace scoped, and reusable", async () => {
  const root = path.resolve(".release/approval-memory-test", String(process.pid));
  await fs.rm(root, { recursive: true, force: true });
  const memory = new ApprovalMemory(root);
  try {
    const saved = await memory.remember("workspace-a", rememberable);
    assert.ok(saved);
    assert.equal((await memory.find("workspace-a", rememberable.approvalKey!))?.actionKey, rememberable.approvalKey);
    assert.equal(await memory.find("workspace-b", rememberable.approvalKey!), null);

    const touched = await memory.touch("workspace-a", rememberable.approvalKey!);
    assert.equal(touched?.useCount, 2);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});

test("critical approvals are never persisted", async () => {
  const root = path.resolve(".release/approval-memory-critical", String(process.pid));
  await fs.rm(root, { recursive: true, force: true });
  const memory = new ApprovalMemory(root);
  try {
    assert.equal(await memory.remember("workspace-a", critical()), null);
    assert.deepEqual(await memory.list("workspace-a"), []);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
