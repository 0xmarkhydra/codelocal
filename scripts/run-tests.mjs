import { readdir } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const directory = path.join(repo, "dist", "__tests__");
const names = (await readdir(directory)).filter((name) => name.endsWith(".test.js")).sort();
if (!names.length) {
  console.error(`No compiled tests found in ${directory}`);
  process.exit(1);
}

const files = names.map((name) => path.join(directory, name));
const result = spawnSync(process.execPath, ["--test", ...files], {
  cwd: repo,
  stdio: "inherit",
  env: process.env,
});

if (result.error) {
  console.error(result.error);
  process.exit(1);
}
process.exit(result.status ?? 1);
