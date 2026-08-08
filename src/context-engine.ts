import { promises as fs } from "node:fs";
import path from "node:path";
import type { SemanticRouter } from "./semantic-router.js";
import { WorkspaceIntelligenceIndex } from "./workspace-index.js";

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
  intelligence: ReturnType<WorkspaceIntelligenceIndex["summary"]>;
};

function unique(values: string[]) {
  return [...new Set(values)];
}

function normalizeLanguage(language: string | null) {
  if (!language) return null;
  if (language === "typescript" || language === "javascript") return "typescript/javascript";
  if (language === "c" || language === "cpp") return "c/c++";
  return language;
}

export class ProjectContextEngine {
  private cached: ProjectMap | null = null;
  private epoch = 0;

  constructor(
    private root: string,
    private semantic: SemanticRouter,
    private intelligence = new WorkspaceIntelligenceIndex(root),
  ) {}

  invalidate(paths?: string[]) {
    this.cached = null;
    this.epoch++;
    this.intelligence.invalidate(paths);
  }

  noteChange(relativePath: string) {
    this.cached = null;
    this.epoch++;
    this.intelligence.noteChange(relativePath);
  }

  indexSummary() {
    return this.intelligence.summary();
  }

  async refresh(force = false) {
    await this.intelligence.ensureFresh(force);
    if (force) this.cached = null;
    return this.intelligence.summary();
  }

  private async packageMetadata(manifests: string[]) {
    const frameworks = new Set<string>();
    const scriptCommands = new Map<string, string[]>();
    const packageFiles = manifests.filter((file) => file.endsWith("package.json")).slice(0, 80);

    for (const relative of packageFiles) {
      try {
        const parsed = JSON.parse(await fs.readFile(path.join(this.root, relative), "utf8"));
        const deps = { ...(parsed?.dependencies ?? {}), ...(parsed?.devDependencies ?? {}) };
        for (const name of ["next", "@nestjs/core", "react", "vue", "@angular/core", "express", "fastify", "svelte", "nuxt", "expo", "react-native"]) {
          if (name in deps) frameworks.add(name);
        }
        const scripts = parsed?.scripts ?? {};
        const dir = path.posix.dirname(relative) === "." ? "." : path.posix.dirname(relative);
        for (const [name] of Object.entries(scripts)) {
          const list = scriptCommands.get(name) ?? [];
          list.push(dir);
          scriptCommands.set(name, list);
        }
      } catch {}
    }

    return { frameworks: [...frameworks], scriptCommands };
  }

  async map(force = false): Promise<ProjectMap> {
    await this.intelligence.ensureFresh(force);
    if (this.cached && !force) return this.cached;

    const records = this.intelligence.allFiles();
    const files = records.map((record) => record.path);
    const fileSet = new Set(files);
    const manifests = records.filter((record) => record.kind === "manifest").map((record) => record.path);
    const lockfiles = files.filter((file) => /(^|\/)(package-lock\.json|pnpm-lock\.yaml|yarn\.lock|bun\.lockb?|poetry\.lock|uv\.lock|Cargo\.lock|go\.sum|composer\.lock|pubspec\.lock|Gemfile\.lock|Podfile\.lock)$/i.test(file));
    const languages = unique(records.map((record) => normalizeLanguage(record.language)).filter((value): value is string => !!value));
    const packageMeta = await this.packageMetadata(manifests);
    const frameworks = [...packageMeta.frameworks];

    for (const relative of manifests.filter((file) => file.endsWith("pyproject.toml")).slice(0, 20)) {
      const py = await fs.readFile(path.join(this.root, relative), "utf8").catch(() => "");
      if (/fastapi/i.test(py)) frameworks.push("fastapi");
      if (/django/i.test(py)) frameworks.push("django");
      if (/flask/i.test(py)) frameworks.push("flask");
    }
    if (manifests.some((file) => file.endsWith("pubspec.yaml"))) frameworks.push("flutter/dart");
    if (files.some((file) => /(^|\/)ios\/Runner\.xcodeproj\//.test(file))) frameworks.push("ios/xcode");

    const packageManager = files.some((file) => /(^|\/)pnpm-lock\.yaml$/.test(file)) ? "pnpm"
      : files.some((file) => /(^|\/)yarn\.lock$/.test(file)) ? "yarn"
      : files.some((file) => /(^|\/)bun\.lockb?$/.test(file)) ? "bun"
      : files.some((file) => /(^|\/)package-lock\.json$/.test(file)) ? "npm"
      : null;

    const scriptCommand = (name: string, dir: string) => {
      const runner = packageManager === "npm" ? `npm run ${name}` : packageManager ? `${packageManager} ${name}` : `npm run ${name}`;
      return dir === "." ? runner : `cd ${dir} && ${runner}`;
    };
    const commandsFor = (pattern: RegExp) => [...packageMeta.scriptCommands.entries()]
      .filter(([name]) => pattern.test(name))
      .flatMap(([name, dirs]) => dirs.map((dir) => scriptCommand(name, dir)));

    const buildCommands = commandsFor(/^(build|compile)(:|$)/);
    const testCommands = commandsFor(/^(test)(:|$)/);
    const lintCommands = commandsFor(/^(lint)(:|$)/);
    const typecheckCommands = commandsFor(/^(typecheck|check)(:|$)/);

    if (manifests.some((file) => file.endsWith("Cargo.toml"))) { buildCommands.push("cargo check"); testCommands.push("cargo test"); }
    if (manifests.some((file) => file.endsWith("go.mod"))) { buildCommands.push("go build ./..."); testCommands.push("go test ./..."); }
    if (manifests.some((file) => /pyproject|requirements/.test(file))) testCommands.push("pytest");
    if (manifests.some((file) => file.endsWith("pubspec.yaml"))) {
      buildCommands.push("flutter analyze");
      testCommands.push("flutter test");
    }

    const workspaceRoots = unique(manifests.map((file) => path.posix.dirname(file)).map((dir) => dir === "." ? "." : dir));
    const rootsFromFiles = unique(files.map((file) => file.split("/")[0]).filter((name) => ["src", "app", "apps", "packages", "lib", "cmd", "internal", "pkg", "crates"].includes(name)));
    const sourceRoots = unique([
      ...rootsFromFiles,
      ...workspaceRoots.flatMap((workspace) => ["lib", "src", "app"].map((name) => workspace === "." ? name : `${workspace}/${name}`).filter((candidate) => files.some((file) => file.startsWith(`${candidate}/`)))),
    ]);
    const testRoots = unique(files.map((file) => file.split("/").slice(0, -1).join("/")).filter((dir) => /(^|\/)(__tests__|test|tests|spec|specs)$/.test(dir))).slice(0, 200);
    const entrypoints = files.filter((file) => /(^|\/)(main|index|server|app)\.(ts|tsx|js|jsx|py|rs|go|java|kt|cs|php|dart|swift)$/i.test(file)).slice(0, 150);
    const instructionFiles = files.filter((file) => /(^|\/)(AGENTS\.md|CLAUDE\.md|copilot-instructions\.md)$/i.test(file));
    const moduleCandidates = new Set<string>();
    for (const rootName of sourceRoots.length ? sourceRoots : ["src", "lib"]) {
      for (const file of files) {
        if (!file.startsWith(`${rootName}/`)) continue;
        const rest = file.slice(rootName.length + 1);
        const first = rest.split("/")[0];
        if (first && !first.includes(".")) moduleCandidates.add(`${rootName}/${first}`);
      }
    }

    this.cached = {
      generatedAt: Date.now(),
      rootName: path.basename(this.root),
      languages,
      frameworks: unique(frameworks),
      workspaceRoots: workspaceRoots.length ? workspaceRoots : ["."],
      entrypoints,
      sourceRoots,
      testRoots,
      manifests,
      lockfiles,
      buildCommands: unique(buildCommands),
      testCommands: unique(testCommands),
      lintCommands: unique(lintCommands),
      typecheckCommands: unique(typecheckCommands),
      instructionFiles,
      modules: [...moduleCandidates].slice(0, 300),
      packageManager,
      intelligence: this.intelligence.summary(),
    };
    return this.cached;
  }

  async relevant(taskHint: string, limit = 30) {
    const project = await this.map();
    await this.intelligence.ensureFresh();
    const rankedFiles = this.intelligence.rank(taskHint, Math.max(limit * 2, 40));
    const terms = unique(taskHint.toLowerCase().split(/[^a-z0-9_$-]+/).filter((value) => value.length >= 3)).slice(0, 10);
    const symbols: any[] = [];

    for (const term of terms) {
      const found = await this.semantic.workspaceSymbols(term, Math.max(6, Math.ceil(limit / Math.max(1, terms.length))));
      symbols.push(...found);
      if (symbols.length >= limit * 2) break;
    }

    const symbolPaths = unique(symbols.map((symbol) => symbol.path).filter(Boolean));
    const rankedPaths = rankedFiles.map((file) => file.path);
    const graphNeighbors = this.intelligence.neighbors([...rankedPaths.slice(0, 12), ...symbolPaths.slice(0, 12)], limit * 2);
    const relevantPaths = unique([...rankedPaths, ...symbolPaths, ...graphNeighbors]).slice(0, limit);
    const rankingByPath = new Map(rankedFiles.map((file) => [file.path, file]));

    return {
      taskHint,
      strategy: "incremental-index + lexical-symbol-ranking + dependency-neighbors + semantic-provider",
      index: this.intelligence.summary(),
      project: {
        languages: project.languages,
        frameworks: project.frameworks,
        workspaceRoots: project.workspaceRoots,
        sourceRoots: project.sourceRoots,
        testRoots: project.testRoots,
        entrypoints: project.entrypoints.slice(0, 30),
        commands: { build: project.buildCommands, test: project.testCommands, lint: project.lintCommands, typecheck: project.typecheckCommands },
      },
      rankedFiles: relevantPaths.map((file) => {
        const ranked = rankingByPath.get(file);
        return ranked ? { path: file, score: ranked.score, reasons: ranked.reasons, language: ranked.language, symbols: ranked.symbols.slice(0, 20), imports: ranked.imports.slice(0, 20), changedAt: ranked.changedAt } : { path: file, score: null, reasons: [symbolPaths.includes(file) ? "semantic-symbol" : "graph-neighbor"] };
      }),
      symbols: symbols.slice(0, limit * 2),
      relevantPaths,
      recommendation: relevantPaths.length
        ? "Read ranked files/symbol ranges first, then expand through graph neighbors only when needed."
        : "Narrow the task hint or use semantic/text search before any broad scan.",
    };
  }
}
