import test from "node:test";
import assert from "node:assert/strict";
import { formatToolTraceContext } from "../log.js";

test("tool trace identifies the MCP session and workspace", () => {
  assert.equal(
    formatToolTraceContext({
      mcpSessionId: "4da89ac4-5638-4f95-b5cf-5b1883e4b588",
      workspaceName: "BIDDI",
      workspaceId: "BIDDI-b71e5501cc",
    }),
    "MCP 4da89ac4 › BIDDI · BIDDI-b71e5501cc",
  );
});

test("tool trace remains readable for legacy calls without a session id", () => {
  assert.equal(
    formatToolTraceContext({ workspaceId: "codex-mcp-e759036dfa" }),
    "MCP legacy › codex-mcp-e759036dfa",
  );
});
