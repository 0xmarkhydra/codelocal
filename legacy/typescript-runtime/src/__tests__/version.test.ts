import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { VERSION } from "../version.js";

test("runtime version stays aligned with package.json", async () => {
  const pkg = JSON.parse(await readFile("package.json", "utf8")) as { version?: string };
  assert.equal(VERSION, pkg.version);
});
