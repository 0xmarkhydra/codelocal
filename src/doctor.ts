import { promises as fs } from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";

const PROJECT_ROOT = process.env.PROJECT_ROOT;
if (!PROJECT_ROOT) {
  console.error("PROJECT_ROOT is required. Example: PROJECT_ROOT=\"$HOME/Desktop/BIDDI\" npm run doctor");
  process.exit(1);
}

const root = path.resolve(PROJECT_ROOT);

async function exists(relative: string) {
  return fs.stat(path.join(root, relative)).then(() => true).catch(() => false);
}

function exec(command: string, args: string[] = [], cwd = root, timeoutMs = 5000) {
  return new Promise<{ ok: boolean; output: string }>((resolve) => {
    const child = spawn(command, args, { cwd, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    const timer = setTimeout(() => child.kill("SIGTERM"), timeoutMs);
    child.stdout.on("data", (d) => { output += d.toString(); });
    child.stderr.on("data", (d) => { output += d.toString(); });
    child.on("error", () => { clearTimeout(timer); resolve({ ok: false, output: "not found" }); });
    child.on("close", (code) => { clearTimeout(timer); resolve({ ok: code === 0, output: output.trim() }); });
  });
}

async function commandVersion(command: string, args = ["--version"]) {
  const result = await exec(command, args, root, 4000);
  return result.ok ? result.output.split("\n")[0] : null;
}

const realRoot = await fs.realpath(root).catch(() => null);
if (!realRoot) {
  console.error(`❌ PROJECT_ROOT does not exist: ${root}`);
  process.exit(1);
}

let packageJson: any = null;
try { packageJson = JSON.parse(await fs.readFile(path.join(realRoot, "package.json"), "utf8")); } catch {}

const markers: Array<[string, string]> = [
  ["package.json", "Node/JavaScript"],
  ["tsconfig.json", "TypeScript"],
  ["pyproject.toml", "Python"],
  ["requirements.txt", "Python"],
  ["Cargo.toml", "Rust"],
  ["go.mod", "Go"],
  ["pom.xml", "Java/Maven"],
  ["build.gradle", "Java/Gradle"],
  ["build.gradle.kts", "Kotlin/Gradle"],
  ["CMakeLists.txt", "C/C++"],
  ["composer.json", "PHP"],
  ["pubspec.yaml", "Dart/Flutter"],
  ["Package.swift", "Swift"],
];

const detected: string[] = [];
for (const [file, language] of markers) if (await exists(file) && !detected.includes(language)) detected.push(language);

let packageManager: string | null = null;
if (await exists("pnpm-lock.yaml")) packageManager = "pnpm";
else if (await exists("yarn.lock")) packageManager = "yarn";
else if (await exists("bun.lockb") || await exists("bun.lock")) packageManager = "bun";
else if (await exists("package-lock.json")) packageManager = "npm";

const tools: Array<[string, string, string[]?]> = [
  ["git", "Git"],
  ["rg", "ripgrep"],
  ["node", "Node.js"],
  ["npm", "npm"],
  ["pnpm", "pnpm"],
  ["yarn", "Yarn"],
  ["bun", "Bun"],
  ["python3", "Python"],
  ["pyright", "Pyright"],
  ["basedpyright", "BasedPyright"],
  ["rustc", "Rust"],
  ["rust-analyzer", "rust-analyzer"],
  ["go", "Go"],
  ["gopls", "gopls", ["version"]],
  ["clangd", "clangd"],
  ["java", "Java", ["-version"]],
  ["sourcekit-lsp", "SourceKit-LSP", ["--version"]],
];

const versions = new Map<string, string | null>();
for (const [cmd, label, args] of tools) versions.set(label, await commandVersion(cmd, args));

const git = await exec("git", ["rev-parse", "--show-toplevel"]);
const branch = await exec("git", ["branch", "--show-current"]);
const gitignore = await exists(".gitignore");
const instructions = ["AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"];
const instructionFiles: string[] = [];
for (const file of instructions) if (await exists(file)) instructionFiles.push(file);

const workspaces = packageJson?.workspaces ?? ((await exists("pnpm-workspace.yaml")) ? "pnpm-workspace.yaml" : null);
const deps = { ...(packageJson?.dependencies ?? {}), ...(packageJson?.devDependencies ?? {}) };
const frameworks = ["next", "react", "@nestjs/core", "vue", "@angular/core", "express", "fastify"].filter((x) => x in deps);

console.log("\nCodeLocal real-project doctor\n");
console.log(`Project:        ${realRoot}`);
console.log(`Languages:      ${detected.length ? detected.join(", ") : "unknown / generic text fallback"}`);
console.log(`Frameworks:     ${frameworks.length ? frameworks.join(", ") : "none detected"}`);
console.log(`Package mgr:    ${packageManager ?? "none detected"}`);
console.log(`Monorepo:       ${workspaces ? "yes" : "no/unknown"}`);
console.log(`Git repo:       ${git.ok ? "yes" : "no"}`);
console.log(`Git branch:     ${branch.ok ? branch.output || "detached/unknown" : "n/a"}`);
console.log(`.gitignore:     ${gitignore ? "yes" : "no"}`);
console.log(`Instructions:   ${instructionFiles.length ? instructionFiles.join(", ") : "none"}`);

console.log("\nLocal capabilities:");
for (const [, label] of tools) {
  const version = versions.get(label);
  if (version) console.log(`  ✅ ${label.padEnd(18)} ${version}`);
}

const warnings: string[] = [];
if (!versions.get("ripgrep")) warnings.push("Install ripgrep (`rg`) for fast code search; CodeLocal will otherwise use a slower fallback.");
if (detected.includes("Python") && !versions.get("Pyright") && !versions.get("BasedPyright")) warnings.push("Python detected but no Pyright/BasedPyright found. Text search works, semantic diagnostics will be limited until installed.");
if (detected.includes("Rust") && !versions.get("rust-analyzer")) warnings.push("Rust detected but rust-analyzer is missing. Install it for semantic navigation.");
if (detected.includes("Go") && !versions.get("gopls")) warnings.push("Go detected but gopls is missing. Install it for semantic navigation.");
if ((detected.includes("C/C++")) && !versions.get("clangd")) warnings.push("C/C++ detected but clangd is missing.");
if (!gitignore) warnings.push("No .gitignore found. Add one so normal retrieval avoids generated/dependency files.");
if (!instructionFiles.length) warnings.push("No AGENTS.md found. Optional: add AGENTS.md with project-specific coding/test conventions.");
if (packageManager && !versions.get(packageManager === "npm" ? "npm" : packageManager === "pnpm" ? "pnpm" : packageManager === "yarn" ? "Yarn" : "Bun")) warnings.push(`Detected ${packageManager} lockfile but ${packageManager} executable was not found.`);

console.log("\nSecurity defaults:");
console.log("  ✅ PROJECT_ROOT boundary enforced by CodeLocal client");
console.log("  ✅ sensitive paths (.env/private keys/credential dirs) blocked");
console.log("  ✅ ignored/generated files excluded from normal retrieval");
console.log("  ✅ risky shell commands require policy/approval when shell is enabled");
console.log("  ⚠️  native OS sandbox is not yet full Codex CLI parity");

if (warnings.length) {
  console.log("\nRecommendations:");
  warnings.forEach((warning) => console.log(`  ⚠️  ${warning}`));
}

console.log("\nResult:");
if (!git.ok) console.log("  ⚠️  Usable, but Git-aware coding workflow will be limited.");
else console.log("  ✅ Workspace is ready for CodeLocal real-project testing.");

console.log("\nNext:");
console.log("  Start the client with the same PROJECT_ROOT and CODELOCAL_ALLOW_SHELL=1.");
