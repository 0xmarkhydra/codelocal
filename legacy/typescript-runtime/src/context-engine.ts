import { promises as fs } from "node:fs";
import path from "node:path";
import type { SemanticRouter, SemanticLocation } from "./semantic-router.js";
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

type ContextSnippet = {
  path: string;
  startLine: number;
  endLine: number;
  totalLines: number;
  content: string;
  reason: string;
};

const MAX_CONTEXT_CHARS = Math.max(8_000, Number(process.env.CODELOCAL_CONTEXT_MAX_CHARS ?? 48_000) || 48_000);
const MAX_SNIPPET_CHARS = Math.max(1_000, Number(process.env.CODELOCAL_CONTEXT_SNIPPET_CHARS ?? 7_000) || 7_000);
const MAX_CONTEXT_FILES = Math.max(2, Number(process.env.CODELOCAL_CONTEXT_FILES ?? 8) || 8);

function unique<T>(values: T[]) {
  return [...new Set(values)];
}

function normalizeLanguage(language: string | null) {
  if (!language) return null;
  if (language === "typescript" || language === "javascript") return "typescript/javascript";
  if (language === "c" || language === "cpp") return "c/c++";
  return language;
}

function taskTerms(taskHint: string) {
  return unique(taskHint.toLowerCase().split(/[^a-z0-9_$-]+/).filter((value) => value.length >= 2)).slice(0, 14);
}

function symbolMatches(symbol: SemanticLocation, terms: string[]) {
  const name = (symbol.name ?? "").toLowerCase();
  const file = symbol.path.toLowerCase();
  return terms.some((term) => name.includes(term) || file.includes(term));
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
    const summary = await this.intelligence.ensureFresh(force);
    if (force) this.cached = null;
    return summary;
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
    const indexSummary = await this.intelligence.ensureFresh(force);
    if (this.cached && !force && this.cached.intelligence.builtAt === indexSummary.builtAt && this.cached.intelligence.dirty === indexSummary.dirty) return this.cached;

    const records = this.intelligence.allFiles();
    const files = records.map((record) => record.path);
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
      intelligence: indexSummary,
    };
    return this.cached;
  }

  private async lspSymbolsForFiles(paths: string[], terms: string[], limit: number) {
    const symbols: SemanticLocation[] = [];
    for (const relative of paths.slice(0, Math.min(10, paths.length))) {
      const document = await this.semantic.documentSymbols(relative, 160).catch(() => [] as SemanticLocation[]);
      const matches = terms.length ? document.filter((symbol) => symbolMatches(symbol, terms)) : document.slice(0, 12);
      symbols.push(...(matches.length ? matches : document.slice(0, 5)));
      if (symbols.length >= limit) break;
    }
    return symbols.slice(0, limit);
  }

  private async diagnosticsForFiles(paths: string[], limit = 40) {
    const diagnostics: any[] = [];
    for (const relative of paths.slice(0, 5)) {
      const values = await this.semantic.diagnostics(relative, 20).catch(() => [] as any[]);
      diagnostics.push(...values.map((item) => ({ ...item, path: item.path ?? relative })));
      if (diagnostics.length >= limit) break;
    }
    return diagnostics.slice(0, limit);
  }

  private async snippet(relativePath: string, preferredLine: number | undefined, reason: string): Promise<ContextSnippet | null> {
    try {
      const absolute = path.resolve(this.root, relativePath);
      const stat = await fs.stat(absolute);
      if (!stat.isFile() || stat.size > 2 * 1024 * 1024) return null;
      const text = await fs.readFile(absolute, "utf8");
      if (text.includes("\u0000")) return null;
      const lines = text.split(/\r?\n/);
      const center = Math.max(1, Math.min(lines.length, preferredLine ?? 1));
      const radius = preferredLine ? 24 : 32;
      let startLine = Math.max(1, center - radius);
      let endLine = Math.min(lines.length, center + radius);
      let content = lines.slice(startLine - 1, endLine).join("\n");
      if (content.length > MAX_SNIPPET_CHARS) {
        content = content.slice(0, MAX_SNIPPET_CHARS);
        const keptLines = content.split(/\r?\n/).length;
        endLine = Math.min(endLine, startLine + keptLines - 1);
      }
      return { path: relativePath, startLine, endLine, totalLines: lines.length, content, reason };
    } catch {
      return null;
    }
  }

  async relevant(taskHint: string, limit = 30) {
    const project = await this.map();
    await this.intelligence.ensureFresh();
    const terms = taskTerms(taskHint);
    const rankedFiles = this.intelligence.rank(taskHint, Math.max(limit * 3, 60));
    const rankedPaths = rankedFiles.map((file) => file.path);

    const workspaceSymbols: SemanticLocation[] = [];
    for (const term of terms.slice(0, 10)) {
      const found = await this.semantic.workspaceSymbols(term, Math.max(8, Math.ceil(limit / Math.max(1, terms.length))));
      workspaceSymbols.push(...found);
      if (workspaceSymbols.length >= limit * 2) break;
    }

    const lspSymbols = await this.lspSymbolsForFiles(rankedPaths, terms, limit * 2);
    const symbols = [...workspaceSymbols, ...lspSymbols].filter((symbol, index, all) =>
      all.findIndex((candidate) => candidate.path === symbol.path && candidate.line === symbol.line && candidate.name === symbol.name) === index,
    );
    const symbolPaths = unique(symbols.map((symbol) => symbol.path).filter(Boolean));
    const graphNeighbors = this.intelligence.neighbors([...rankedPaths.slice(0, 12), ...symbolPaths.slice(0, 12)], limit * 3);
    const relevantPaths = unique([...rankedPaths, ...symbolPaths, ...graphNeighbors]).slice(0, limit);
    const rankingByPath = new Map(rankedFiles.map((file) => [file.path, file]));
    const diagnostics = await this.diagnosticsForFiles(relevantPaths);

    const relevantSet = new Set(relevantPaths);
    const graphEdges = this.intelligence.graph(5000)
      .filter((edge) => relevantSet.has(edge.from) || relevantSet.has(edge.to))
      .slice(0, 120);

    const snippets: ContextSnippet[] = [];
    let usedChars = 0;
    for (const relative of relevantPaths.slice(0, MAX_CONTEXT_FILES)) {
      const symbol = symbols.find((item) => item.path === relative && symbolMatches(item, terms)) ?? symbols.find((item) => item.path === relative);
      const diagnostic = diagnostics.find((item) => item.path === relative);
      const ranked = rankingByPath.get(relative);
      const preferredLine = symbol?.line ?? diagnostic?.line;
      const reason = ranked?.reasons?.join(", ") || (symbol ? "semantic-symbol" : diagnostic ? "diagnostic" : "graph-neighbor");
      const value = await this.snippet(relative, preferredLine, reason);
      if (!value) continue;
      if (usedChars + value.content.length > MAX_CONTEXT_CHARS) break;
      snippets.push(value);
      usedChars += value.content.length;
    }

    return {
      taskHint,
      strategy: "persistent-incremental-index + per-root-LSP + semantic-symbols + dependency-neighbors + recent-change-ranking + bounded-source-snippets",
      epoch: this.epoch,
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
        return ranked
          ? { path: file, score: ranked.score, reasons: ranked.reasons, language: ranked.language, symbols: ranked.symbols.slice(0, 20), imports: ranked.imports.slice(0, 20), changedAt: ranked.changedAt }
          : { path: file, score: null, reasons: [symbolPaths.includes(file) ? "semantic-symbol" : "graph-neighbor"] };
      }),
      symbols: symbols.slice(0, limit * 2),
      diagnostics,
      graphEdges,
      snippets,
      relevantPaths,
      contextBudget: { maxChars: MAX_CONTEXT_CHARS, usedChars, maxFiles: MAX_CONTEXT_FILES },
      recommendation: relevantPaths.length
        ? "Use this packet first. Ask for exact definitions/references or read a larger range only when the packet is insufficient; all side effects remain separate MCP calls."
        : "Narrow the task hint or use semantic/text search before any broad scan.",
    };
  }
}
