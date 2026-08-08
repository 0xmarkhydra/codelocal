import assert from "node:assert/strict";
import test from "node:test";
import os from "node:os";
import path from "node:path";
import { promises as fs } from "node:fs";
import { McpHub } from "../mcp-hub.js";

test("MCP connect guard blocks probe before a stdio transport is started", async () => {
  const stateDir = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-mcp-guard-state-"));
  const workspace = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-mcp-guard-workspace-"));
  const previousState = process.env.CODELOCAL_STATE_DIR;
  process.env.CODELOCAL_STATE_DIR = stateDir;
  let guarded = 0;
  const hub = new McpHub(workspace, async (config) => {
    guarded++;
    assert.equal(config.name, "blocked-test");
    throw new Error("local approval required");
  });
  try {
    await hub.addServer({
      name: "blocked-test",
      enabled: true,
      scope: "workspace",
      workspaceRoot: workspace,
      transport: "stdio",
      command: process.execPath,
      args: [path.resolve("scripts/test-mcp-server.mjs")],
    });
    await assert.rejects(() => hub.probe("blocked-test"), /local approval required/);
    assert.equal(guarded, 1);
  } finally {
    await hub.shutdown();
    if (previousState == null) delete process.env.CODELOCAL_STATE_DIR;
    else process.env.CODELOCAL_STATE_DIR = previousState;
    await fs.rm(stateDir, { recursive: true, force: true });
    await fs.rm(workspace, { recursive: true, force: true });
  }
});
