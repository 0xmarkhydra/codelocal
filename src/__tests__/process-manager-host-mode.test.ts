import test from "node:test";
import assert from "node:assert/strict";
import { isMacDeveloperHostCommand } from "../process-manager.js";

test("macOS developer host mode never accepts shell composition", () => {
  for (const command of [
    "flutter doctor && echo surprise",
    "dart analyze; echo surprise",
    "xcodebuild | tee output.txt",
    "pod install > /tmp/pod.log",
    "flutter test $(echo surprise)",
    "flutter test\necho surprise",
  ]) {
    assert.equal(isMacDeveloperHostCommand(command), false, command);
  }
});

test("simple developer command is eligible only on macOS", () => {
  assert.equal(isMacDeveloperHostCommand("flutter analyze"), process.platform === "darwin");
  assert.equal(isMacDeveloperHostCommand("fvm flutter test"), process.platform === "darwin");
  assert.equal(isMacDeveloperHostCommand("FOO=bar dart analyze"), process.platform === "darwin");
});
