import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { WorkspaceIntelligenceIndex } from "../workspace-index.js";

test("workspace intelligence ranks matching symbols and graph neighbors", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "codelocal-index-"));
  try {
    await mkdir(path.join(root, "src"), { recursive: true });
    await writeFile(path.join(root, "package.json"), JSON.stringify({ scripts: { test: "node --test" } }));
    await writeFile(path.join(root, "src", "settings.ts"), [
      'import { saveUser } from "./user";',
      "export function updateSettings() { return saveUser(); }",
    ].join("\n"));
    await writeFile(path.join(root, "src", "user.ts"), "export function saveUser() { return true; }\n");
    await writeFile(path.join(root, "src", "unrelated.ts"), "export function ping() { return 'pong'; }\n");

    const index = new WorkspaceIntelligenceIndex(root, 1000, 6, 128 * 1024, 1);
    await index.ensureFresh(true);
    const ranked = index.rank("fix updateSettings saveUser", 10);
    assert.equal(ranked[0]?.path, "src/settings.ts");
    assert.ok(ranked.some((file) => file.path === "src/user.ts"));
    assert.ok(index.neighbors(["src/settings.ts"]).includes("src/user.ts"));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("workspace intelligence refreshes files changed outside CodeLocal", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "codelocal-index-refresh-"));
  try {
    await mkdir(path.join(root, "lib"), { recursive: true });
    const target = path.join(root, "lib", "feature.dart");
    await writeFile(target, "class OldFeature {}\n");
    const index = new WorkspaceIntelligenceIndex(root, 1000, 6, 128 * 1024, 1);
    await index.ensureFresh(true);
    assert.ok(index.rank("OldFeature", 5).some((file) => file.path === "lib/feature.dart"));

    await new Promise((resolve) => setTimeout(resolve, 5));
    await writeFile(target, "class NewFeature {}\n");
    await index.ensureFresh(true);
    assert.ok(index.rank("NewFeature", 5).some((file) => file.path === "lib/feature.dart"));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
