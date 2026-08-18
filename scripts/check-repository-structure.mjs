import { access, readFile, readdir } from "node:fs/promises";
import { constants } from "node:fs";
import path from "node:path";
import process from "node:process";

const root = process.cwd();
const failures = [];

const exists = async (relative) => {
  try {
    await access(path.join(root, relative), constants.F_OK);
    return true;
  } catch {
    return false;
  }
};

const rootEntries = await readdir(root, { withFileTypes: true });
for (const entry of rootEntries) {
  if (!entry.isFile()) continue;
  if (/_(?:MASTER_)?PLAN\.md$/i.test(entry.name) || /_PLAN\.md$/i.test(entry.name)) {
    failures.push(`plan document must live under docs/plans/: ${entry.name}`);
  }
}

for (const required of [
  "AGENTS.md",
  "docs/architecture/REPOSITORY_STRUCTURE.md",
  "docs/plans",
  "docs/guides",
  "docs/operations",
  "docs/integrations",
  "src/LEGACY_RUNTIME.md",
  "src/AGENTS.md",
  "internal/ui/AGENTS.md",
  "internal/cloud/AGENTS.md",
  "internal/cloudserver/AGENTS.md",
  "internal/automation/AGENTS.md",
  "internal/mcpgateway/AGENTS.md",
  "internal/project/AGENTS.md",
]) {
  if (!(await exists(required))) failures.push(`required repository boundary missing: ${required}`);
}

for (const forbidden of [
  "1.5.0",
  "docs/plugin-submission",
  "docs/USER_GUIDE.md",
  "docs/HOW_CODELOCAL_WORKS.md",
  "docs/SECURITY_AND_PRIVACY.md",
  "docs/BETA_CHANNEL.md",
  "docs/MIGRATION_ROLLOUT.md",
]) {
  if (await exists(forbidden)) failures.push(`obsolete repository location still exists: ${forbidden}`);
}

const pkg = JSON.parse(await readFile(path.join(root, "package.json"), "utf8"));
const scripts = Object.values(pkg.scripts ?? {}).join("\n");
for (const legacyEntrypoint of ["src/index.ts", "src/client-v2", "src/server-saas", "node src/"]) {
  if (scripts.includes(legacyEntrypoint)) {
    failures.push(`root package script must not execute legacy runtime: ${legacyEntrypoint}`);
  }
}

if (failures.length > 0) {
  console.error("Repository structure check failed:");
  for (const failure of failures) console.error(`- ${failure}`);
  process.exit(1);
}

console.log("Repository structure check passed.");
