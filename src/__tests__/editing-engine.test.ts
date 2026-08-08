import test from "node:test";
import assert from "node:assert/strict";
import { promises as fs } from "node:fs";
import os from "node:os";
import path from "node:path";
import { createHash } from "node:crypto";
import { EditingEngine } from "../editing-engine.js";

function hash(text: string) {
  return createHash("sha256").update(text).digest("hex");
}

test("applies multiple validated file edits", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-edit-"));
  try {
    await fs.writeFile(path.join(root, "a.txt"), "hello world\n", "utf8");
    await fs.writeFile(path.join(root, "b.txt"), "one two\n", "utf8");
    const engine = new EditingEngine(root);
    const result = await engine.applyEdits([
      { path: "a.txt", expectedHash: hash("hello world\n"), edits: [{ startOffset: 6, endOffset: 11, replacement: "CodeLocal" }] },
      { path: "b.txt", expectedHash: hash("one two\n"), edits: [{ startLine: 1, startColumn: 5, endLine: 1, endColumn: 8, replacement: "three" }] },
    ]);
    assert.equal(result.changed.length, 2);
    assert.equal(await fs.readFile(path.join(root, "a.txt"), "utf8"), "hello CodeLocal\n");
    assert.equal(await fs.readFile(path.join(root, "b.txt"), "utf8"), "one three\n");
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});

test("rejects stale hash without mutating file", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-edit-"));
  try {
    await fs.writeFile(path.join(root, "a.txt"), "current\n", "utf8");
    const engine = new EditingEngine(root);
    await assert.rejects(() => engine.applyEdits([{ path: "a.txt", expectedHash: hash("stale\n"), edits: [{ startOffset: 0, endOffset: 7, replacement: "oops" }] }]), /changed since read/);
    assert.equal(await fs.readFile(path.join(root, "a.txt"), "utf8"), "current\n");
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});

test("rejects overlapping edits", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-edit-"));
  try {
    await fs.writeFile(path.join(root, "a.txt"), "abcdefgh\n", "utf8");
    const engine = new EditingEngine(root);
    await assert.rejects(() => engine.applyEdits([{ path: "a.txt", edits: [
      { startOffset: 1, endOffset: 5, replacement: "x" },
      { startOffset: 4, endOffset: 7, replacement: "y" },
    ] }]), /Overlapping edits/);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
