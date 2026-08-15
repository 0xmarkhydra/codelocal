import test from "node:test";
import assert from "node:assert/strict";
import { estimateMcpTokens, formatTerminalTimestamp, formatToolTraceContext } from "../log.js";

test("tool trace shows workspace without exposing the MCP session id", () => {
  assert.equal(
    formatToolTraceContext({
      mcpSessionId: "4da89ac4-5638-4f95-b5cf-5b1883e4b588",
      workspaceName: "BIDDI",
      workspaceId: "BIDDI-b71e5501cc",
    }),
    "BIDDI · BIDDI-b71e5501cc",
  );
});

test("tool trace remains readable with only a workspace id", () => {
  assert.equal(formatToolTraceContext({ workspaceId: "codex-mcp-e759036dfa" }), "codex-mcp-e759036dfa");
});

test("terminal timestamp uses local MM/DD/YYYY - HH:mm format", () => {
  assert.equal(formatTerminalTimestamp(new Date(2026, 7, 31, 12, 15)), "08/31/2026 - 12:15");
});

test("MCP token estimate uses the same four-unicode-characters approximation", () => {
  assert.equal(estimateMcpTokens("12345678"), 3);
});
