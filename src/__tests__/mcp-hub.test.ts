import assert from "node:assert/strict";
import test from "node:test";
import os from "node:os";
import path from "node:path";
import { promises as fs } from "node:fs";
import { McpHub, mcpStatePaths } from "../mcp-hub.js";
import { bridgeMcpToolResult } from "../mcp-bridge.js";

async function withHub(run: (hub: McpHub, workspace: string, stateDir: string) => Promise<void>) {
  const stateDir = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-mcp-state-"));
  const workspace = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-mcp-workspace-"));
  const previous = process.env.CODELOCAL_STATE_DIR;
  process.env.CODELOCAL_STATE_DIR = stateDir;
  const hub = new McpHub(workspace);
  try {
    await run(hub, workspace, stateDir);
  } finally {
    await hub.shutdown();
    if (previous == null) delete process.env.CODELOCAL_STATE_DIR;
    else process.env.CODELOCAL_STATE_DIR = previous;
    await fs.rm(stateDir, { recursive: true, force: true });
    await fs.rm(workspace, { recursive: true, force: true });
  }
}

test("MCP Hub installs, probes, searches and calls a stdio MCP without exposing its tools globally", async () => {
  await withHub(async (hub, workspace) => {
    const fixture = path.resolve("scripts/test-mcp-server.mjs");
    await hub.addServer({
      name: "echo",
      enabled: true,
      scope: "workspace",
      workspaceRoot: workspace,
      transport: "stdio",
      command: process.execPath,
      args: [fixture],
    });

    const probe = await hub.probe("echo");
    assert.equal(probe.toolCount, 1);
    assert.equal(probe.tools[0]?.name, "echo_message");

    const search = await hub.searchTools("echo message");
    assert.equal(search.results[0]?.server, "echo");
    assert.equal(search.results[0]?.tool, "echo_message");

    const info = await hub.toolInfo("echo", "echo_message");
    assert.equal(info.server, "echo");
    assert.equal(info.name, "echo_message");
    assert.equal(typeof info.inputSchema, "object");

    const called = await hub.callTool("echo", "echo_message", { message: "hello CodeLocal" });
    const content = (called.result.content ?? []) as Array<{ type?: string; text?: string }>;
    assert.equal(content.find((item) => item.type === "text")?.text, "hello CodeLocal");

    const listed = await hub.listServers();
    assert.equal(listed.length, 1);
    assert.equal(listed[0]?.toolsCached, 1);
  });
});

test("MCP Hub stores environment references instead of secret values", async () => {
  await withHub(async (hub, workspace) => {
    process.env.CODELOCAL_TEST_SECRET = "do-not-persist-this-secret";
    await hub.addServer({
      name: "secret-ref",
      enabled: true,
      scope: "workspace",
      workspaceRoot: workspace,
      transport: "stdio",
      command: process.execPath,
      args: ["--version"],
      env: { API_TOKEN: { source: "CODELOCAL_TEST_SECRET" } },
    });
    const registry = await fs.readFile(mcpStatePaths().registry, "utf8");
    assert.match(registry, /CODELOCAL_TEST_SECRET/);
    assert.doesNotMatch(registry, /do-not-persist-this-secret/);
    delete process.env.CODELOCAL_TEST_SECRET;
  });
});

test("MCP result bridge preserves text, images and structured content", () => {
  const bridged = bridgeMcpToolResult({
    server: "demo",
    tool: "capture",
    result: {
      content: [
        { type: "text", text: "done" },
        { type: "image", data: "aGVsbG8=", mimeType: "image/png" },
      ],
      structuredContent: { ok: true },
    },
  });
  assert.ok(bridged);
  assert.equal((bridged!.content as any[]).length, 2);
  assert.deepEqual(bridged!.structuredContent, { ok: true });
});
