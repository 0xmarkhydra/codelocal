import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { resolveLspRoot, type LspServerSpec } from "../lsp.js";

const dart: LspServerSpec = {
  id: "dart-analyzer",
  command: "dart",
  args: ["language-server", "--protocol=lsp"],
  languages: ["dart"],
  extensions: [".dart"],
  rootMarkers: ["pubspec.yaml"],
};

test("LSP root resolver selects nearest nested project marker", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "codelocal-lsp-root-"));
  try {
    const app = path.join(root, "apps", "mobile");
    const nested = path.join(app, "packages", "feature");
    await mkdir(path.join(nested, "lib"), { recursive: true });
    await writeFile(path.join(app, "pubspec.yaml"), "name: mobile\n");
    await writeFile(path.join(nested, "pubspec.yaml"), "name: feature\n");
    const file = path.join(nested, "lib", "feature.dart");
    await writeFile(file, "class Feature {}\n");

    assert.equal(await resolveLspRoot(root, file, dart), nested);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("LSP root resolver falls back to workspace root", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "codelocal-lsp-fallback-"));
  try {
    const file = path.join(root, "lib", "feature.dart");
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, "class Feature {}\n");
    assert.equal(await resolveLspRoot(root, file, dart), root);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
