import { promises as fs } from "node:fs";
import path from "node:path";
import type { SemanticRouter } from "./semantic-router.js";
import { isSensitivePath } from "./security-policy.js";

export type ProjectMap = {
  generatedAt: number;
  rootName: string;
  languages: string[];
  frameworks: string[];
  workspaceRoots: string[];
  entrypoints: string[];
  sourceRoots: string[];
  testRoots: string[];
  manifests: string[];
  lockfiles: string[];
  buildCommands: string[];
  testCommands: string[];
  lintCommands: string[];
  typecheckCommands: string[];
  instructionFiles: string[];
  modules: string[];
  packageManager: string | null;
};

const SKIP = new Set([".git", "node_modules", ".next", "dist", "build", "target", ".venv", "venv", "coverage", ".cache", ".turbo"]);

async function exists(file: string) {
  return fs.stat(file).then(() => true).catch(() => false);
}

function unique(values: string[]) {
  return [...new Set(values)];
}

export class ProjectContextEngine {
  private cached: ProjectMap | null = null;
  private epoch = 0;

  constructor(private root: string, private semantic: SemanticRouter) {}

  invalidate() {
    this.cached = null;
    this.epoch++;
  }

  private async discoverFiles(maxDepth = 4, maxEntries = 6000) {
    const out: string[] = [];
    const walk = async (dir: string, depth: number) => {
      if (out.length >= maxEntries || depth > maxDepth) return;
      let entries: Awaited<ReturnType<typeof fs.readdir>>;
      try { entries = await fs.readdir(dir, { withFileTypes: true }) as any; } catch { return; }
      for (const entry of entries as any[]) {
        if (out.length >= maxEntries) break;
        if (entry.isSymbolicLink()) continue;
        if (entry.isDirectory() && SKIP.has(entry.name)) continue;
        const absolute = path.join(dir, entry.name);
        const relative = path.relative(this.root, absolute).split(path.sep).join("/");
        if (isSensitivePath(relative)) continue;
        if (entry.isDirectory()) await walk(absolute, depth + 1);
        else out.push(relative);
      }
    };
    await walk(this.root, 0);
    return out;
  }

  async map(force = false): Promise<ProjectMap> {
    if (this.cached && !force) return this.cached;
    const files = await this.discoverFiles();
    const fileSet = new Set(files);
    const manifests = files.filter((f) => /(^|\/)(package\.json|pyproject\.toml|requirements[^/]*\.txt|Cargo\.toml|go\.mod|pom\.xml|build\.gradle(?:\.kts)?|[^/]+\.csproj|CMakeLists\.txt|composer\.json|pubspec\.yaml|Package\.swift)$/i.test(f));
    const lockfiles = files.filter((f) => /(^|\/)(package-lock\.json|pnpm-lock\.yaml|yarn\.lock|bun\.lockb?|poetry\.lock|uv\.lock|Cargo\.lock|go\.sum|composer\.lock|pubspec\.lock|Gemfile\.lock)$/i.test(f));
    const languages: string[] = [];
    if (files.some((f) => /\.(ts|tsx|js|jsx|mts|cts|mjs|cjs)$/.test(f))) languages.push("typescript/javascript");
    if (files.some((f) => f.endsWith(".py")) || manifests.some((f) => /pyproject|requirements/.test(f))) languages.push("python");
    if (files.some((f) => f.endsWith(".rs")) || manifests.some((f) => f.endsWith("Cargo.toml"))) languages.push("rust");
    if (files.some((f) => f.endsWith(".go")) || manifests.some((f) => f.endsWith("go.mod"))) languages.push("go");
    if (files.some((f) => /\.(c|cc|cpp|cxx|h|hpp)$/.test(f))) languages.push("c/c++");
    if (files.some((f) => f.endsWith(".java"))) languages.push("java");
    if (files.some((f) => /\.kts?$/.test(f))) languages.push("kotlin");
    if (files.some((f) => f.endsWith(".cs"))) languages.push("csharp");
    if (files.some((f) => f.endsWith(".php"))) languages.push("php");
    if (files.some((f) => f.endsWith(".swift"))) languages.push("swift");
    if (files.some((f) => f.endsWith(".dart"))) languages.push("dart");
    if (files.some((f) => f.endsWith(".rb"))) languages.push("ruby");
    if (files.some((f) => f.endsWith(".lua"))) languages.push("lua");
    if (files.some((f) => /\.exs?$/.test(f))) languages.push("elixir");
    if (files.some((f) => f.endsWith(".zig"))) languages.push("zig");
    if (files.some((f) => f.endsWith(".sol"))) languages.push("solidity");

    let packageJson: any = null;
    try { packageJson = JSON.parse(await fs.readFile(path.join(this.root, "package.json"), "utf8")); } catch {}
    const deps = { ...(packageJson?.dependencies ?? {}), ...(packageJson?.devDependencies ?? {}) };
    const frameworks = ["next", "@nestjs/core", "react", "vue", "@angular/core", "express", "fastify", "svelte", "nuxt"].filter((x) => x in deps);
    if (fileSet.has("pyproject.toml")) {
      const py = await fs.readFile(path.join(this.root, "pyproject.toml"), "utf8").catch(() => "");
      if (/fastapi/i.test(py)) frameworks.push("fastapi");
      if (/django/i.test(py)) frameworks.push("django");
      if (/flask/i.test(py)) frameworks.push("flask");
    }

    const packageManager = fileSet.has("pnpm-lock.yaml") ? "pnpm" : fileSet.has("yarn.lock") ? "yarn" : files.some((f) => /(^|\/)bun\.lockb?$/.test(f)) ? "bun" : fileSet.has("package-lock.json") ? "npm" : null;
    const scripts = packageJson?.scripts ?? {};
    const scriptCommand = (name: string) => packageManager === "npm" ? `npm run ${name}` : packageManager ? `${packageManager} ${name}` : `npm run ${name}`;
    const buildCommands = Object.keys(scripts).filter((k) => /^(build|compile)(:|$)/.test(k)).map(scriptCommand);
    const testCommands = Object.keys(scripts).filter((k) => /^(test)(:|$)/.test(k)).map(scriptCommand);
    const lintCommands = Object.keys(scripts).filter((k) => /^(lint)(:|$)/.test(k)).map(scriptCommand);
    const typecheckCommands = Object.keys(scripts).filter((k) => /^(typecheck|check)(:|$)/.test(k)).map(scriptCommand);
    if (manifests.some((f) => f.endsWith("Cargo.toml"))) { buildCommands.push("cargo check"); testCommands.push("cargo test"); }
    if (manifests.some((f) => f.endsWith("go.mod"))) { buildCommands.push("go build ./..."); testCommands.push("go test ./..."); }
    if (manifests.some((f) => /pyproject|requirements/.test(f))) testCommands.push("pytest");

    const sourceRoots = unique(files.map((f) => f.split("/")[0]).filter((x) => ["src", "app", "apps", "packages", "lib", "cmd", "internal", "pkg", "crates"].includes(x)));
    const testRoots = unique(files.map((f) => f.split("/")[0]).filter((x) => ["test", "tests", "__tests__", "spec", "specs"].includes(x)));
    const workspaceRoots = unique(manifests.map((f) => path.posix.dirname(f)).map((x) => x === "." ? "." : x));
    const entrypoints = files.filter((f) => /(^|\/)(main|index|server|app)\.(ts|tsx|js|jsx|py|rs|go|java|kt|cs|php)$/i.test(f)).slice(0, 100);
    const instructionFiles = files.filter((f) => /(^|\/)(AGENTS\.md|CLAUDE\.md|copilot-instructions\.md)$/i.test(f));
    const moduleCandidates = new Set<string>();
    for (const rootName of sourceRoots.length ? sourceRoots : ["src"]) {
      for (const file of files) {
        if (!file.startsWith(`${rootName}/`)) continue;
        const rest = file.slice(rootName.length + 1);
        const first = rest.split("/")[0];
        if (first && !first.includes(".")) moduleCandidates.add(`${rootName}/${first}`);
      }
    }

    this.cached = {
      generatedAt: Date.now(), rootName: path.basename(this.root), languages: unique(languages), frameworks: unique(frameworks),
      workspaceRoots: workspaceRoots.length ? workspaceRoots : ["."], entrypoints, sourceRoots, testRoots, manifests, lockfiles,
      buildCommands: unique(buildCommands), testCommands: unique(testCommands), lintCommands: unique(lintCommands), typecheckCommands: unique(typecheckCommands),
      instructionFiles, modules: [...moduleCandidates].slice(0, 200), packageManager,
    };
    return this.cached;
  }

  async relevant(taskHint: string, limit = 30) {
    const project = await this.map();
    const terms = unique(taskHint.toLowerCase().split(/[^a-z0-9_$-]+/).filter((x) => x.length >= 3)).slice(0, 8);
    const symbols: any[] = [];
    for (const term of terms) {
      const found = await this.semantic.workspaceSymbols(term, Math.max(5, Math.ceil(limit / Math.max(1, terms.length))));
      symbols.push(...found);
      if (symbols.length >= limit) break;
    }
    const relevantPaths = unique(symbols.map((x) => x.path).filter(Boolean)).slice(0, limit);
    return {
      taskHint,
      project: {
        languages: project.languages,
        frameworks: project.frameworks,
        sourceRoots: project.sourceRoots,
        testRoots: project.testRoots,
        entrypoints: project.entrypoints.slice(0, 20),
        commands: { build: project.buildCommands, test: project.testCommands, lint: project.lintCommands, typecheck: project.typecheckCommands },
      },
      symbols: symbols.slice(0, limit),
      relevantPaths,
      recommendation: relevantPaths.length ? "Read targeted symbols/ranges before broad repository scans." : "Use semantic/text search for a more specific task term before broad listing.",
    };
  }
}
