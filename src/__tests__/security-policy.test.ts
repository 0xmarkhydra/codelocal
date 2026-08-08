import test from "node:test";
import assert from "node:assert/strict";
import { classifyCommand, isSensitivePath, redactCommand } from "../security-policy.js";

test("blocks privilege escalation and credential paths", () => {
  assert.equal(classifyCommand("sudo rm -rf /tmp/demo").blocked, true);
  assert.equal(classifyCommand("cat ~/.ssh/id_rsa").blocked, true);
  assert.equal(classifyCommand("dd if=/dev/zero of=/dev/disk1").blocked, true);
});

test("requires approval for package and git writes", () => {
  const install = classifyCommand("npm install axios");
  assert.equal(install.requiresApproval, true);
  assert.equal(install.blocked, false);
  const push = classifyCommand("git push origin main");
  assert.equal(push.requiresApproval, true);
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
