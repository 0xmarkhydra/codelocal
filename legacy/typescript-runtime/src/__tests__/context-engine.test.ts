import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { ProjectContextEngine } from "../context-engine.js";
import { WorkspaceIntelligenceIndex } from "../workspace-index.js";

test("context engine returns ranked snippets without autonomous side effects", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "codelocal-context-"));
  try {
    await mkdir(path.join(root, "lib"), { recursive: true });
    await writeFile(path.join(root, "pubspec.yaml"), "name: demo\n");
    await writeFile(path.join(root, "lib", "settings_page.dart"), [
      "import './profile_repository.dart';",
      "class SettingsPage {",
      "  Future<void> saveAvatar() async {",
      "    await ProfileRepository().saveAvatar();",
      "  }",
      "}",
    ].join("\n"));
    await writeFile(path.join(root, "lib", "profile_repository.dart"), [
      "class ProfileRepository {",
      "  Future<void> saveAvatar() async {}",
      "}",
    ].join("\n"));

    const semantic = {
      workspaceSymbols: async (query: string) => query.toLowerCase().includes("avatar")
        ? [{ path: "lib/settings_page.dart", line: 3, column: 16, name: "saveAvatar", provider: "fake-lsp" }]
        : [],
      documentSymbols: async (file: string) => file.endsWith("settings_page.dart")
        ? [{ path: file, line: 3, column: 16, name: "saveAvatar", provider: "fake-lsp" }]
        : [{ path: file, line: 2, column: 16, name: "saveAvatar", provider: "fake-lsp" }],
      diagnostics: async () => [],
    } as any;

    const intelligence = new WorkspaceIntelligenceIndex(root, 1000, 6, 128 * 1024, 1);
    const engine = new ProjectContextEngine(root, semantic, intelligence);
    const packet = await engine.relevant("settings avatar save bug", 10);

    assert.ok(packet.relevantPaths.includes("lib/settings_page.dart"));
    assert.ok(packet.snippets.some((item) => item.path === "lib/settings_page.dart" && item.content.includes("saveAvatar")));
    assert.ok(packet.symbols.some((item) => item.name === "saveAvatar"));
    assert.ok(packet.contextBudget.usedChars > 0);
    assert.match(packet.strategy, /per-root-LSP/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
