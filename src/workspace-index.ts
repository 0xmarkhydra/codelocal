import { promises as fs } from "node:fs";
import path from "node:path";
import os from "node:os";
import { createHash, randomUUID } from "node:crypto";
import { isSensitivePath } from "./security-policy.js";

export type IndexedFile = {
  path: string;
  size: number;
  mtimeMs: number;
  language: string | null;
  kind: "source" | "test" | "manifest" | "config" | "other";
  imports: string[];
  symbols: string[];
  tokens: string[];
  packageName: string | null;
  changedAt: number | null;
};

export type RankedFile = IndexedFile & {
  score: number;
  reasons: string[];
};

const SKIP_DIRS = new Set([
  ".git", "node_modules", ".next", "dist", "build", "target", ".venv", "venv", "coverage",
  ".cache", ".turbo", ".dart_tool", ".idea", ".gradle", "Pods", "DerivedData",
]);

const MANIFEST_RE = /(^|\/)(package\.json|pyproject\.toml|requirements[^/]*\.txt|Cargo\.toml|go\.mod|pom\.xml|build\.gradle(?:\.kts)?|[^/]+\.csproj|CMakeLists\.txt|composer\.json|pubspec\.yaml|Package\.swift|Podfile)$/i;
const CONFIG_RE = /(^|\/)(tsconfig[^/]*\.json|jsconfig[^/]*\.json|analysis_options\.yaml|\.eslintrc(?:\.[^/]+)?|eslint\.config\.[^/]+|vite\.config\.[^/]+|next\.config\.[^/]+)$/i;
const TEST_RE = /(^|\/)(__tests__|test|tests|spec|specs)(\/|$)|\.(test|spec)\.[^.]+$/i;

const LANGUAGE_BY_EXT: Record<string, string> = {
  ".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
  ".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
  ".py": "python", ".rs": "rust", ".go": "go", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp", ".hpp": "cpp",
  ".java": "java", ".kt": "kotlin", ".kts": "kotlin", ".cs": "csharp", ".php": "php", ".rb": "ruby", ".lua": "lua",
  ".swift": "swift", ".dart": "dart", ".ex": "elixir", ".exs": "elixir", ".zig": "zig", ".sol": "solidity",
};

function normalizeRelative(value: string) {
  return value.replace(/\\/g, "/").replace(/^\.\//, "");
}

function tokenize(value: string) {
  return [...new Set(value.toLowerCase().split(/[^a-z0-9_$.-]+/).flatMap((part) => part.split(/[._/-]+/)).filter((part) => part.length >= 2))];
}

function languageFor(file: string) {
  return LANGUAGE_BY_EXT[path.extname(file).toLowerCase()] ?? null;
}

function kindFor(file: string, language: string | null): IndexedFile["kind"] {
  if (MANIFEST_RE.test(file)) return "manifest";
  if (CONFIG_RE.test(file)) return "config";
  if (TEST_RE.test(file)) return "test";
  return language ? "source" : "other";
}

function parseImports(text: string, language: string | null) {
  const out = new Set<string>();
  const patterns: RegExp[] = [];
  if (language === "typescript" || language === "javascript") {
    patterns.push(/\b(?:import|export)\b[^"'`]*?\bfrom\s*["'`]([^"'`]+)["'`]/g, /\brequire\s*\(\s*["'`]([^"'`]+)["'`]\s*\)/g, /\bimport\s*\(\s*["'`]([^"'`]+)["'`]\s*\)/g);
  } else if (language === "dart") {
    patterns.push(/\b(?:import|export|part)\s+["']([^"']+)["']/g);
  } else if (language === "python") {
    patterns.push(/^\s*from\s+([A-Za-z0-9_.]+)\s+import\b/gm, /^\s*import\s+([A-Za-z0-9_.]+)/gm);
  } else if (language === "rust") {
    patterns.push(/\buse\s+([A-Za-z0-9_:]+)/g, /\bmod\s+([A-Za-z0-9_]+)/g);
  } else if (language === "go") {
    patterns.push(/\bimport\s+(?:\([^)]*?["`]([^"`]+)["`]|["`]([^"`]+)["`])/gs);
  } else if (language === "swift") {
    patterns.push(/^\s*import\s+([A-Za-z0-9_.]+)/gm);
  }
  for (const pattern of patterns) {
    pattern.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = pattern.exec(text))) {
      const value = match.slice(1).find(Boolean);
      if (value) out.add(value);
      if (out.size >= 200) break;
    }
  }
  return [...out];
}

function parseSymbols(text: string, language: string | null) {
  const out = new Set<string>();
  const patterns: RegExp[] = [
    /\b(?:class|interface|enum|type|struct|trait|mixin|extension|module)\s+([A-Za-z_$][\w$]*)/g,
    /\b(?:function|def|fn|func)\s+([A-Za-z_$][\w$]*)/g,
    /\b(?:const|let|var|final)\s+([A-Za-z_$][\w$]*)\s*(?=[=:])/g,
  ];
  if (language === "dart") patterns.push(/\b(?:Future<[^>]+>|Future|Widget|void|int|double|String|bool|dynamic|[A-Z][A-Za-z0-9_<>, ?]*)\s+([a-zA-Z_$][\w$]*)\s*\(/g);
  for (const pattern of patterns) {
    pattern.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = pattern.exec(text))) {
      if (match[1]) out.add(match[1]);
      if (out.size >= 300) break;
    }
  }
  return [...out];
}

function parsePackageName(file: string, text: string) {
  if (file.endsWith("pubspec.yaml")) {
    return /^\s*name\s*:\s*["']?([A-Za-z0-9_-]+)["']?\s*$/m.exec(text)?.[1] ?? null;
  }
  if (file.endsWith("package.json")) {
    try {
      const value = JSON.parse(text)?.name;
      return typeof value === "string" && value.trim() ? value.trim() : null;
    } catch {}
  }
  return null;
}

function resolveRelativeImport(from: string, specifier: string, known: Set<string>) {
  if (!specifier.startsWith(".")) return null;
  const base = normalizeRelative(path.posix.normalize(path.posix.join(path.posix.dirname(from), specifier)));
  const candidates = [
    base,
    ...Object.keys(LANGUAGE_BY_EXT).map((ext) => `${base}${ext}`),
    ...Object.keys(LANGUAGE_BY_EXT).map((ext) => `${base}/index${ext}`),
  ];
  return candidates.find((candidate) => known.has(candidate)) ?? null;
}

export class WorkspaceIntelligenceIndex {
  private files = new Map<string, IndexedFile>();
  private reverseImports = new Map<string, Set<string>>();
  private packageRoots = new Map<string, string>();
  private recentChanges = new Map<string, number>();
  private builtAt = 0;
  private lastScanAt = 0;
  private dirty = true;
  private scanPromise: Promise<void> | null = null;
  private cacheLoaded = false;
  private readonly cacheDir: string;
  private readonly cacheFile: string;

  constructor(
    private root: string,
    private maxEntries = Math.max(1000, Number(process.env.CODELOCAL_INDEX_MAX_FILES ?? 12000) || 12000),
    private maxDepth = Math.max(2, Number(process.env.CODELOCAL_INDEX_MAX_DEPTH ?? 8) || 8),
    private maxReadBytes = Math.max(16_384, Number(process.env.CODELOCAL_INDEX_MAX_FILE_BYTES ?? 384 * 1024) || 384 * 1024),
    private freshnessMs = Math.max(500, Number(process.env.CODELOCAL_INDEX_FRESHNESS_MS ?? 1500) || 1500),
  ) {
    this.cacheDir = path.join(os.homedir(), ".codelocal", "indexes");
    const key = createHash("sha256").update(path.resolve(root)).digest("hex").slice(0, 24);
    this.cacheFile = path.join(this.cacheDir, `${key}.json`);
  }

  invalidate(paths?: string[]) {
    this.dirty = true;
    if (paths?.length) for (const file of paths) this.noteChange(file);
  }

  noteChange(relativePath: string) {
    const normalized = normalizeRelative(relativePath);
    if (!normalized || normalized === "." || normalized.startsWith("../")) return;
    this.recentChanges.set(normalized, Date.now());
    this.dirty = true;
  }

  private async loadCache() {
    if (this.cacheLoaded) return;
    this.cacheLoaded = true;
    try {
      const parsed = JSON.parse(await fs.readFile(this.cacheFile, "utf8"));
      if (parsed?.version !== 2 || parsed?.root !== path.resolve(this.root) || !Array.isArray(parsed?.files)) return;
      const restored = new Map<string, IndexedFile>();
      for (const raw of parsed.files) {
        if (!raw || typeof raw.path !== "string" || isSensitivePath(raw.path)) continue;
        restored.set(raw.path, { ...raw, packageName: typeof raw.packageName === "string" ? raw.packageName : null } as IndexedFile);
      }
      this.files = restored;
      this.builtAt = Number(parsed.builtAt ?? 0);
      if (Array.isArray(parsed.recentChanges)) {
        for (const item of parsed.recentChanges) {
          if (item && typeof item.file === "string" && Number.isFinite(item.changedAt)) this.recentChanges.set(item.file, Number(item.changedAt));
        }
      }
      this.rebuildGraphMetadata();
    } catch {}
  }

  private async persistCache() {
    try {
      await fs.mkdir(this.cacheDir, { recursive: true, mode: 0o700 });
      const payload = JSON.stringify({
        version: 2,
        root: path.resolve(this.root),
        builtAt: this.builtAt,
        files: [...this.files.values()],
        recentChanges: [...this.recentChanges.entries()].map(([file, changedAt]) => ({ file, changedAt })),
      });
      const temp = `${this.cacheFile}.${process.pid}.${randomUUID()}.tmp`;
      await fs.writeFile(temp, payload, { encoding: "utf8", mode: 0o600 });
      await fs.rename(temp, this.cacheFile);
    } catch {}
  }

  private async discover() {
    const out: Array<{ path: string; size: number; mtimeMs: number }> = [];
    const walk = async (dir: string, depth: number) => {
      if (out.length >= this.maxEntries || depth > this.maxDepth) return;
      let entries: Awaited<ReturnType<typeof fs.readdir>>;
      try { entries = await fs.readdir(dir, { withFileTypes: true }) as any; } catch { return; }
      for (const entry of entries as any[]) {
        if (out.length >= this.maxEntries) break;
        if (entry.isSymbolicLink()) continue;
        if (entry.isDirectory() && SKIP_DIRS.has(entry.name)) continue;
        const absolute = path.join(dir, entry.name);
        const relative = normalizeRelative(path.relative(this.root, absolute));
        if (!relative || isSensitivePath(relative)) continue;
        if (entry.isDirectory()) {
          await walk(absolute, depth + 1);
          continue;
        }
        try {
          const stat = await fs.stat(absolute);
          out.push({ path: relative, size: stat.size, mtimeMs: stat.mtimeMs });
        } catch {}
      }
    };
    await walk(this.root, 0);
    return out;
  }

  private async indexOne(meta: { path: string; size: number; mtimeMs: number }, previous?: IndexedFile) {
    const language = languageFor(meta.path);
    const kind = kindFor(meta.path, language);
    const unchanged = !!previous && previous.size === meta.size && previous.mtimeMs === meta.mtimeMs;
    const changedAt = this.recentChanges.get(meta.path) ?? (!unchanged && previous ? Date.now() : previous?.changedAt ?? null);
    if (unchanged && previous) return { ...previous, changedAt };

    if (!unchanged && previous) this.recentChanges.set(meta.path, changedAt ?? Date.now());
    let text = "";
    if ((language || kind === "manifest" || kind === "config") && meta.size <= this.maxReadBytes) {
      try { text = await fs.readFile(path.join(this.root, meta.path), "utf8"); } catch {}
    }
    const symbols = text ? parseSymbols(text, language) : [];
    const imports = text ? parseImports(text, language) : [];
    const packageName = text && kind === "manifest" ? parsePackageName(meta.path, text) : null;
    const tokens = [...new Set([...tokenize(meta.path), ...symbols.flatMap(tokenize), ...imports.flatMap(tokenize), ...(packageName ? tokenize(packageName) : [])])].slice(0, 800);
    return { ...meta, language, kind, imports, symbols, tokens, packageName, changedAt } satisfies IndexedFile;
  }

  private async scan() {
    await this.loadCache();
    const discovered = await this.discover();
    const next = new Map<string, IndexedFile>();
    for (const meta of discovered) {
      next.set(meta.path, await this.indexOne(meta, this.files.get(meta.path)));
    }
    for (const oldPath of this.files.keys()) {
      if (!next.has(oldPath)) this.recentChanges.set(oldPath, Date.now());
    }
    this.files = next;
    this.rebuildGraphMetadata();
    this.builtAt = Date.now();
    this.lastScanAt = this.builtAt;
    this.dirty = false;
    const cutoff = Date.now() - 10 * 60_000;
    for (const [file, at] of this.recentChanges) if (at < cutoff) this.recentChanges.delete(file);
    await this.persistCache();
  }

  private rebuildGraphMetadata() {
    this.reverseImports.clear();
    this.packageRoots.clear();
    for (const record of this.files.values()) {
      if (!record.packageName) continue;
      if (record.path.endsWith("pubspec.yaml") || record.path.endsWith("package.json")) {
        this.packageRoots.set(record.packageName, path.posix.dirname(record.path) === "." ? "" : path.posix.dirname(record.path));
      }
    }
    const known = new Set(this.files.keys());
    for (const record of this.files.values()) {
      for (const specifier of record.imports) {
        const resolved = this.resolveImport(record.path, specifier, known);
        if (!resolved) continue;
        const set = this.reverseImports.get(resolved) ?? new Set<string>();
        set.add(record.path);
        this.reverseImports.set(resolved, set);
      }
    }
  }

  private resolveImport(from: string, specifier: string, known = new Set(this.files.keys())) {
    const relative = resolveRelativeImport(from, specifier, known);
    if (relative) return relative;

    const dartPackage = /^package:([^/]+)\/(.+)$/.exec(specifier);
    if (dartPackage) {
      const packageRoot = this.packageRoots.get(dartPackage[1]);
      if (packageRoot !== undefined) {
        const candidate = normalizeRelative(path.posix.join(packageRoot, "lib", dartPackage[2]));
        if (known.has(candidate)) return candidate;
      }
    }
    return null;
  }

  async ensureFresh(force = false) {
    await this.loadCache();
    const now = Date.now();
    if (!force && !this.dirty && now - this.lastScanAt < this.freshnessMs) return this.summary();
    if (!force && this.dirty && this.lastScanAt > 0 && now - this.lastScanAt < Math.min(this.freshnessMs, 750)) return this.summary();
    if (!this.scanPromise) {
      this.scanPromise = this.scan().finally(() => { this.scanPromise = null; });
    }
    await this.scanPromise;
    return this.summary();
  }

  allFiles() {
    return [...this.files.values()];
  }

  summary() {
    const files = [...this.files.values()];
    return {
      builtAt: this.builtAt,
      dirty: this.dirty,
      files: files.length,
      sourceFiles: files.filter((file) => file.kind === "source" || file.kind === "test").length,
      manifests: files.filter((file) => file.kind === "manifest").length,
      packages: [...this.packageRoots.entries()].slice(0, 100).map(([name, root]) => ({ name, root: root || "." })),
      languages: [...new Set(files.map((file) => file.language).filter(Boolean))],
      recentChanges: [...this.recentChanges.entries()].sort((a, b) => b[1] - a[1]).slice(0, 30).map(([file, changedAt]) => ({ file, changedAt })),
      persistentCache: this.cacheFile,
    };
  }

  neighbors(paths: string[], limit = 100) {
    const out = new Set<string>();
    const known = new Set(this.files.keys());
    for (const input of paths) {
      const record = this.files.get(input);
      if (record) {
        for (const specifier of record.imports) {
          const resolved = this.resolveImport(record.path, specifier, known);
          if (resolved) out.add(resolved);
          if (out.size >= limit) return [...out];
        }
      }
      for (const importer of this.reverseImports.get(input) ?? []) {
        out.add(importer);
        if (out.size >= limit) return [...out];
      }
    }
    return [...out];
  }

  rank(taskHint: string, limit = 40): RankedFile[] {
    const terms = tokenize(taskHint).filter((term) => term.length >= 2).slice(0, 16);
    const wantsTests = terms.some((term) => ["test", "tests", "spec", "bug", "fix", "regression"].includes(term));
    const now = Date.now();
    const ranked: RankedFile[] = [];

    for (const file of this.files.values()) {
      let score = 0;
      const reasons: string[] = [];
      const lowerPath = file.path.toLowerCase();
      const basename = path.posix.basename(lowerPath);
      const symbolLower = file.symbols.map((value) => value.toLowerCase());
      const importLower = file.imports.map((value) => value.toLowerCase());

      for (const term of terms) {
        if (basename.includes(term)) { score += 18; reasons.push(`filename:${term}`); }
        else if (lowerPath.includes(term)) { score += 10; reasons.push(`path:${term}`); }
        if (symbolLower.some((value) => value === term)) { score += 24; reasons.push(`symbol=${term}`); }
        else if (symbolLower.some((value) => value.includes(term))) { score += 14; reasons.push(`symbol:${term}`); }
        if (importLower.some((value) => value.includes(term))) { score += 7; reasons.push(`import:${term}`); }
        if (file.packageName?.toLowerCase().includes(term)) { score += 8; reasons.push(`package:${term}`); }
        if (file.tokens.includes(term)) score += 3;
      }

      if (file.kind === "manifest" || file.kind === "config") score += terms.length ? 1 : 4;
      if (file.kind === "test") score += wantsTests ? 8 : -2;
      if (/(^|\/)(main|index|app|server)\.[^.]+$/i.test(file.path)) score += 3;
      if (file.changedAt) {
        const age = now - file.changedAt;
        if (age < 5 * 60_000) { score += 14; reasons.push("recently-changed"); }
        else if (age < 30 * 60_000) { score += 7; reasons.push("recent-change"); }
      }
      if (!terms.length && (file.kind === "source" || file.kind === "manifest")) score += 1;
      if (score > 0) ranked.push({ ...file, score, reasons: [...new Set(reasons)].slice(0, 8) });
    }

    ranked.sort((a, b) => b.score - a.score || b.mtimeMs - a.mtimeMs || a.path.localeCompare(b.path));
    const seeds = ranked.slice(0, Math.min(12, ranked.length)).map((item) => item.path);
    const neighbors = new Set(this.neighbors(seeds, 120));
    for (const item of ranked) {
      if (neighbors.has(item.path)) {
        item.score += 6;
        item.reasons = [...new Set([...item.reasons, "graph-neighbor"])];
      }
    }
    ranked.sort((a, b) => b.score - a.score || b.mtimeMs - a.mtimeMs || a.path.localeCompare(b.path));
    return ranked.slice(0, limit);
  }

  graph(limit = 3000) {
    const known = new Set(this.files.keys());
    const edges: Array<{ from: string; to: string; specifier: string }> = [];
    for (const record of this.files.values()) {
      for (const specifier of record.imports) {
        const resolved = this.resolveImport(record.path, specifier, known);
        if (resolved) edges.push({ from: record.path, to: resolved, specifier });
        if (edges.length >= limit) return edges;
      }
    }
    return edges;
  }
}
