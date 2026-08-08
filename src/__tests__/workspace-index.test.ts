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
    await writeFile(path.join(root, "package.json"), JSON.stringify({ name: "demo", scripts: { test: "node --test" } }));
    await writeFile(path.join(root, "src", "settings.ts"), [
      'import { saveUser } from "./user";',
      "export function updateSettings() { return saveUser(); }",
    ].join("\n"));
    await writeFile(path.join(root, "src", "user.ts"), "export function saveUser() { return true; }\n");
    await writeFile(path.join(root, "src", "unrelated.ts"), "export function ping() { return 'pong'; }\n");

    const index = new WorkspaceIntelligenceIndex(root, 1000, 6, 128 * 1024, 1);
    await index.ensureFresh(true);
    const ranked = index.rank("fix updateSettings saveUser", 10);
    const settings = ranked.find((file) => file.path === "src/settings.ts");
    const user = ranked.find((file) => file.path === "src/user.ts");
    assert.ok(settings, "task seed file should be ranked");
    assert.ok(user, "dependency file should be ranked");
    assert.ok(settings.reasons.some((reason) => reason.includes("updatesettings")), "seed should carry symbol evidence");
    assert.ok(user.reasons.includes("graph-neighbor"), "dependency should receive graph-neighbor evidence");
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

test("workspace intelligence resolves local Flutter package imports", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "codelocal-index-dart-package-"));
  try {
    const app = path.join(root, "apps", "mobile");
    await mkdir(path.join(app, "lib", "features"), { recursive: true });
    await writeFile(path.join(app, "pubspec.yaml"), "name: biddi_mobile\n");
    await writeFile(path.join(app, "lib", "settings.dart"), "import 'package:biddi_mobile/features/profile.dart';\nclass Settings {}\n");
    await writeFile(path.join(app, "lib", "features", "profile.dart"), "class ProfileRepository {}\n");

    const index = new WorkspaceIntelligenceIndex(root, 1000, 8, 128 * 1024, 1);
    await index.ensureFresh(true);
    const settings = "apps/mobile/lib/settings.dart";
    const profile = "apps/mobile/lib/features/profile.dart";
    assert.ok(index.neighbors([settings]).includes(profile));
    assert.ok(index.graph().some((edge) => edge.from === settings && edge.to === profile));
    assert.ok(index.summary().packages.some((item) => item.name === "biddi_mobile" && item.root === "apps/mobile"));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
