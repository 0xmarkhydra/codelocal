import assert from "node:assert/strict";
import test from "node:test";
import os from "node:os";
import path from "node:path";
import { promises as fs } from "node:fs";
import { hashPassword, verifyPassword } from "../saas-auth.js";
import { syncCloudMcpInstallations } from "../mcp-cloud-sync.js";
import { mcpStatePaths } from "../mcp-hub.js";

test("SaaS passwords are salted and verified without storing plaintext", async () => {
  const password = "correct horse battery staple";
  const first = await hashPassword(password);
  const second = await hashPassword(password);
  assert.notEqual(first.salt, second.salt);
  assert.notEqual(first.hash, second.hash);
  assert.equal(await verifyPassword(password, first.salt, first.hash), true);
  assert.equal(await verifyPassword("wrong password", first.salt, first.hash), false);
  assert.equal(first.hash.includes(password), false);
});

test("cloud MCP sync persists secret references but never secret values", async () => {
  const previousState = process.env.CODELOCAL_STATE_DIR;
  const previousSecret = process.env.CODELOCAL_TEST_CLOUD_SECRET;
  const stateDir = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-saas-state-"));
  const workspace = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-saas-workspace-"));
  process.env.CODELOCAL_STATE_DIR = stateDir;
  process.env.CODELOCAL_TEST_CLOUD_SECRET = "super-secret-value-that-must-not-be-written";
  try {
    await syncCloudMcpInstallations(workspace, [{
      id: "cloud-install-1",
      name: "cloud-echo",
      enabled: true,
      scope: "workspace",
      workspaceId: "demo",
      transport: "stdio",
      config: { command: process.execPath, args: ["--version"] },
      requiredSecrets: ["CODELOCAL_TEST_CLOUD_SECRET"],
      updatedAt: Date.now(),
    }]);
    const registry = await fs.readFile(mcpStatePaths().registry, "utf8");
    assert.match(registry, /CODELOCAL_TEST_CLOUD_SECRET/);
    assert.doesNotMatch(registry, /super-secret-value-that-must-not-be-written/);
    assert.match(registry, /cloudInstallationId/);
  } finally {
    if (previousState == null) delete process.env.CODELOCAL_STATE_DIR; else process.env.CODELOCAL_STATE_DIR = previousState;
    if (previousSecret == null) delete process.env.CODELOCAL_TEST_CLOUD_SECRET; else process.env.CODELOCAL_TEST_CLOUD_SECRET = previousSecret;
    await fs.rm(stateDir, { recursive: true, force: true });
    await fs.rm(workspace, { recursive: true, force: true });
  }
});
