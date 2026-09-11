import test from "node:test";
import assert from "node:assert/strict";
import { ChatApprovalBroker } from "../chat-approval.js";
import type { PolicyDecision } from "../security-policy.js";

const review: PolicyDecision = {
  riskLevel: "REVIEW",
  matchedRules: ["Git write action"],
  requiresApproval: true,
  blocked: false,
  redactedCommand: "git push origin dev",
  reason: "Git write action",
  approvalPolicy: "rememberable",
  approvalKey: "git.push:origin:dev",
  approvalLabel: "Git push origin dev",
};

test("chat approval token is bound to command/cwd and can be consumed once", () => {
  const broker = new ChatApprovalBroker(60_000);
  const preflight = broker.preflight("git push origin dev", ".", review);
  assert.equal(preflight.status, "approval_required");
  assert.ok(preflight.approvalToken);

  assert.equal(broker.consume(preflight.approvalToken, "git push origin main", ".", review), false);

  const retry = broker.preflight("git push origin dev", ".", review);
  assert.ok(retry.approvalToken);
  assert.equal(broker.consume(retry.approvalToken, "git push origin dev", "subdir", review), false);

  const approved = broker.preflight("git push origin dev", ".", review);
  assert.ok(approved.approvalToken);
  assert.equal(broker.consume(approved.approvalToken, "git push origin dev", ".", review), true);
  assert.equal(broker.consume(approved.approvalToken, "git push origin dev", ".", review), false);
});

test("safe and blocked decisions never create approval credentials", () => {
  const broker = new ChatApprovalBroker();
  const safe: PolicyDecision = { ...review, riskLevel: "SAFE", matchedRules: [], requiresApproval: false, redactedCommand: "npm test", reason: "safe", approvalPolicy: "none", approvalKey: undefined };
  const blocked: PolicyDecision = { ...review, riskLevel: "BLOCKED", blocked: true, requiresApproval: false, redactedCommand: "sudo rm -rf /", reason: "blocked", approvalPolicy: "blocked", approvalKey: undefined };
  assert.deepEqual(broker.preflight("npm test", ".", safe).status, "safe");
  assert.equal(broker.preflight("npm test", ".", safe).approvalToken, undefined);
  assert.equal(broker.preflight("sudo rm -rf /", ".", blocked).status, "blocked");
  assert.equal(broker.preflight("sudo rm -rf /", ".", blocked).approvalToken, undefined);
});
