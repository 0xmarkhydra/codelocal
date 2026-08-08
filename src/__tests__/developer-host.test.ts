import test from "node:test";
import assert from "node:assert/strict";
import { classifyCommand } from "../security-policy.js";

test("flutter and Xcode toolchains require review approval", () => {
  for (const command of [
    "flutter run --flavor dev",
    "cd app-dev && flutter run --flavor dev",
    "dart pub get",
    "xcodebuild -workspace Runner.xcworkspace",
    "xcrun simctl list devices",
    "pod install",
  ]) {
    const decision = classifyCommand(command, "approval");
    assert.equal(decision.blocked, false, command);
    assert.equal(decision.requiresApproval, true, command);
    assert.ok(decision.matchedRules.includes("macOS developer toolchain host execution"), command);
  }
});

test("dangerous commands remain blocked even if developer tools are mentioned", () => {
  const decision = classifyCommand("sudo flutter run --flavor dev", "approval");
  assert.equal(decision.blocked, true);
  assert.equal(decision.riskLevel, "BLOCKED");
});
