import test from "node:test";
import assert from "node:assert/strict";
import { classifyCommand, isSensitivePath, redactCommand } from "../security-policy.js";

test("blocks privilege escalation and credential paths", () => {
  assert.equal(classifyCommand("sudo rm -rf /tmp/demo").blocked, true);
  assert.equal(classifyCommand("cat ~/.ssh/id_rsa").blocked, true);
  assert.equal(classifyCommand("dd if=/dev/zero of=/dev/disk1").blocked, true);
});

test("rememberable actions are scoped to structured developer operations", () => {
  const install = classifyCommand("npm install axios");
  assert.equal(install.requiresApproval, true);
  assert.equal(install.approvalPolicy, "rememberable");
  assert.ok(install.approvalKey?.startsWith("dependency:npm:"));

  const push = classifyCommand("git push origin dev");
  assert.equal(push.requiresApproval, true);
  assert.equal(push.approvalPolicy, "rememberable");
  assert.equal(push.approvalKey, "git.push:origin:dev");

  const testRun = classifyCommand("npm test");
  assert.equal(testRun.requiresApproval, true);
  assert.equal(testRun.approvalPolicy, "rememberable");
  assert.ok(testRun.approvalKey?.startsWith("workspace-exec:"));

  const nodeScript = classifyCommand("node scripts/check.js");
  assert.equal(nodeScript.requiresApproval, true);
  assert.equal(nodeScript.approvalPolicy, "rememberable");
  assert.ok(nodeScript.approvalKey?.startsWith("workspace-exec:"));

  for (const command of ["bash scripts/check.sh", "./scripts/check.sh", "flutter test", "xcodebuild -scheme Demo build", "dotnet test"]) {
    const execution = classifyCommand(command);
    assert.equal(execution.requiresApproval, true, command);
    assert.equal(execution.approvalPolicy, "rememberable", command);
    assert.ok(execution.approvalKey?.startsWith("workspace-exec:"), command);
  }
});

test("explicit executable paths outside an authorized workspace are blocked", () => {
  const context = { workspaceRoot: "/work/project", cwd: "/work/project" };
  assert.equal(classifyCommand("/tmp/untrusted-tool", "approval", context).blocked, true);
  assert.equal(classifyCommand("./scripts/check.sh", "approval", context).blocked, false);
});

test("critical Git mutations always require fresh approval", () => {
  for (const command of [
    "git push --force origin dev",
    "git push --force-with-lease=refs/heads/dev origin dev",
    "git push origin :main",
    "git push --delete origin main",
    "git commit --amend -m fix",
  ]) {
    const decision = classifyCommand(command);
    assert.equal(decision.requiresApproval, true, command);
    assert.equal(decision.approvalPolicy, "always", command);
    assert.equal(decision.riskLevel, "CRITICAL", command);
    assert.equal(decision.approvalKey, undefined, command);
  }
});

test("shell composition cannot inherit a remembered command grant", () => {
  const decision = classifyCommand("npm test && rm -rf build");
  assert.equal(decision.requiresApproval, true);
  assert.equal(decision.approvalPolicy, "always");
  assert.equal(decision.riskLevel, "CRITICAL");
  assert.equal(decision.approvalKey, undefined);
});

test("network deny blocks network commands", () => {
  const decision = classifyCommand("curl https://example.com", "deny");
  assert.equal(decision.blocked, true);
});

test("redacts representative secrets", () => {
  const command = 'curl -H "Authorization: Bearer abc123" https://example.com?x=1';
  assert.equal(redactCommand(command).includes("abc123"), false);
  assert.equal(redactCommand("OPENAI_API_KEY=sk-test npm test").includes("sk-test"), false);
});

test("sensitive path detection is independent from gitignore", () => {
  assert.equal(isSensitivePath(".env"), true);
  assert.equal(isSensitivePath("config/.env.production"), true);
  assert.equal(isSensitivePath(".env.example"), false);
  assert.equal(isSensitivePath("src/app.ts"), false);
});
