import test from "node:test";
import assert from "node:assert/strict";
import { classifyCommand, redactCommand } from "../security-policy.js";

test("routine developer commands run without approval", () => {
  for (const command of [
    "flutter analyze",
    "flutter test",
    "flutter run --flavor dev",
    "fvm flutter doctor",
    "dart analyze",
    "dart format .",
    "xcodebuild -scheme Runner -configuration Debug",
    "xcrun simctl list devices",
  ]) {
    const decision = classifyCommand(command, "approval");
    assert.equal(decision.blocked, false, command);
    assert.equal(decision.requiresApproval, false, command);
  }
});

test("dependency mutations and publishing still require chat approval", () => {
  for (const command of [
    "flutter pub add http",
    "dart pub publish",
    "pod install",
    "npm install lodash",
    "npm publish",
  ]) {
    const decision = classifyCommand(command, "approval");
    assert.equal(decision.blocked, false, command);
    assert.equal(decision.requiresApproval, true, command);
  }
});

test("composed shell commands cannot inherit routine developer safety", () => {
  const chained = classifyCommand("flutter doctor && rm -rf build", "approval");
  assert.equal(chained.requiresApproval, true);
  assert.ok(chained.matchedRules.some((rule) => rule.includes("recursive delete") || rule.includes("composed shell")));

  const hidden = classifyCommand("flutter analyze && sudo rm -rf /", "approval");
  assert.equal(hidden.blocked, true);
  assert.equal(hidden.riskLevel, "BLOCKED");
});

test("terminal log redaction removes common inline secrets", () => {
  const value = redactCommand("GITHUB_TOKEN=secret123 curl -H 'Authorization: Bearer abc123' https://example.com");
  assert.doesNotMatch(value, /secret123|abc123/);
  assert.match(value, /\[REDACTED\]/);
});
