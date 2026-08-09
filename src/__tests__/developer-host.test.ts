import test from "node:test";
import assert from "node:assert/strict";
import { classifyCommand } from "../security-policy.js";

test("routine Flutter and Xcode commands do not require chat approval", () => {
  for (const command of [
    "flutter run --flavor dev",
    "flutter analyze",
    "flutter test",
    "xcodebuild -workspace Runner.xcworkspace",
    "xcrun simctl list devices",
  ]) {
    const decision = classifyCommand(command, "approval");
    assert.equal(decision.blocked, false, command);
    assert.equal(decision.requiresApproval, false, command);
  }
});

test("developer dependency mutations and shell composition still require review", () => {
  for (const command of [
    "cd app-dev && flutter run --flavor dev",
    "dart pub get",
    "pod install",
  ]) {
    const decision = classifyCommand(command, "approval");
    assert.equal(decision.blocked, false, command);
    assert.equal(decision.requiresApproval, true, command);
  }
});

test("dangerous commands remain blocked even if developer tools are mentioned", () => {
  const decision = classifyCommand("sudo flutter run --flavor dev", "approval");
  assert.equal(decision.blocked, true);
  assert.equal(decision.riskLevel, "BLOCKED");
});
