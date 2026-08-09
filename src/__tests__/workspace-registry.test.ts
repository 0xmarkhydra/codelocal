import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, realpath, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { WorkspaceRegistry, workspaceIdForPath } from "../workspace-registry.js";

test("workspace registry grants, remembers and revokes a project folder", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "codelocal-workspaces-"));
  try {
    const project = path.join(temp, "My Project");
    await mkdir(project);
    const canonicalProject = await realpath(project);
    const registry = new WorkspaceRegistry(path.join(temp, "state", "workspaces.json"));

    const first = await registry.grant(project);
    const second = await registry.grant(project);
    assert.equal(first.workspaceId, second.workspaceId);
    assert.equal(first.workspaceId, workspaceIdForPath(canonicalProject));
    assert.equal(first.workspaceName, "My Project");
    assert.equal(first.grantedAt, second.grantedAt);

    const listed = await registry.list();
    assert.equal(listed.length, 1);
    assert.equal(listed[0].localPath, canonicalProject);

    assert.equal(await registry.revoke(first.workspaceId), true);
    assert.deepEqual(await registry.list(), []);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});

test("workspace registry refuses a whole home directory grant", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "codelocal-workspaces-"));
  try {
    const registry = new WorkspaceRegistry(path.join(temp, "workspaces.json"));
    await assert.rejects(() => registry.grant(os.homedir()), /entire home directory/i);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});
