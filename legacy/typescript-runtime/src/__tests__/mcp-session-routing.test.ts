import test from "node:test";
import assert from "node:assert/strict";
import { createWorkspaceRoutingState, selectWorkspaceForSession, workspaceKeyForSession } from "../mcp-session-routing.js";

test("workspace selection remains isolated across interleaved MCP sessions", () => {
  const threadA = createWorkspaceRoutingState();
  const threadB = createWorkspaceRoutingState();

  selectWorkspaceForSession(threadA, "user::mac::codex-mcp");
  selectWorkspaceForSession(threadB, "user::mac::BIDDI");

  for (let call = 0; call < 20; call++) {
    assert.equal(workspaceKeyForSession(threadA), "user::mac::codex-mcp");
    assert.equal(workspaceKeyForSession(threadB), "user::mac::BIDDI");
  }
});

test("explicit routing overrides one call without changing session selection", () => {
  const session = createWorkspaceRoutingState();
  selectWorkspaceForSession(session, "user::mac::codex-mcp");

  assert.equal(workspaceKeyForSession(session, "user::mac::BIDDI"), "user::mac::BIDDI");
  assert.equal(workspaceKeyForSession(session), "user::mac::codex-mcp");
});

test("a fresh MCP session starts without inheriting another session selection", () => {
  const existing = createWorkspaceRoutingState();
  selectWorkspaceForSession(existing, "user::mac::codex-mcp");

  const fresh = createWorkspaceRoutingState();
  assert.equal(workspaceKeyForSession(fresh), null);
  assert.equal(workspaceKeyForSession(existing), "user::mac::codex-mcp");
});
