import test from "node:test";
import assert from "node:assert/strict";
import { PROTOCOL_VERSION, protocolCompatible, normalizeError, isSideEffectingTool } from "../protocol.js";

test("protocol compatibility accepts current and legacy minimum", () => {
  assert.equal(protocolCompatible(PROTOCOL_VERSION), true);
  assert.equal(protocolCompatible(1), true);
  assert.equal(protocolCompatible(0), false);
  assert.equal(protocolCompatible(PROTOCOL_VERSION + 1), false);
});

test("error normalization produces stable codes", () => {
  assert.equal(normalizeError(new Error("Access blocked by sensitive-path policy: .env")).errorCode, "SENSITIVE_PATH");
  assert.equal(normalizeError(new Error("File changed since read. hash mismatch")).errorCode, "CONFLICT");
  assert.equal(normalizeError(new Error("Command denied by local approval policy")).errorCode, "APPROVAL_DENIED");
});

test("side effect set covers write and process operations", () => {
  assert.equal(isSideEffectingTool("write_file"), true);
  assert.equal(isSideEffectingTool("git_commit"), true);
  assert.equal(isSideEffectingTool("read_file"), false);
});
