import path from "node:path";
import { fileURLToPath } from "node:url";
import { promises as fs } from "node:fs";
import { spawn } from "node:child_process";
import { TypeScriptSemanticIndex } from "./semantic.js";
import { LspClient, commandExists, resolveLspRoot, type LspServerSpec } from "./lsp.js";
import { isSensitivePath } from "./security-policy.js";

export type SemanticLocation = {
  path: string;
  line: number;
  column: number;
  endLine?: number;
  endColumn?: number;
  name?: string;
  kind?: string;
  provider?: string;
};

const SPECS: LspServerSpec[] = [
  { id: "pyright", command: "pyright-langserver", args: ["--stdio"], languages: ["python"], extensions: [".py"], rootMarkers: ["pyproject.toml", "setup.cfg", "setup.py", "requirements.txt"] },
  { id: "rust-analyzer", command: "rust-analyzer", args: [], languages: ["rust"], extensions: [".rs"], rootMarkers: ["Cargo.toml"] },
  { id: "gopls", command: "gopls", args: [], languages: ["go"], extensions: [".go"], rootMarkers: ["go.mod", "go.work"] },
  { id: "clangd", command: "clangd", args: ["--background-index"], languages: ["c", "cpp"], extensions: [".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh"], rootMarkers: ["compile_commands.json", "CMakeLists.txt"] },
  { id: "jdtls", command: "jdtls", args: [], languages: ["java"], extensions: [".java"], rootMarkers: ["pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"] },
  { id: "kotlin-language-server", command: "kotlin-language-server", args: [], languages: ["kotlin"], extensions: [".kt", ".kts"], rootMarkers: ["build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "pom.xml"] },
  { id: "lua-language-server", command: "lua-language-server", args: [], languages: ["lua"], extensions: [".lua"], rootMarkers: [".luarc.json", ".luarc.jsonc"] },
  { id: "sourcekit-lsp", command: "sourcekit-lsp", args: [], languages: ["swift"], extensions: [".swift"], rootMarkers: ["Package.swift"] },
  { id: "dart-analyzer", command: "dart", args: ["language-server", "--protocol=lsp"], languages: ["dart"], extensions: [".dart"], rootMarkers: ["pubspec.yaml"] },
  { id: "zls", command: "zls", args: [], languages: ["zig"], extensions: [".zig"], rootMarkers: ["build.zig"] },
];

function rel(root: string, file: string) {
  return path.relative(root, file).split(path.sep).join("/") || ".";
}

function lspUriToPath(uri: string) {
  try { return fileURLToPath(uri); } catch { return uri; }
}

function locationFromLsp(root: string, value: any, provider: string): SemanticLocation | null {
  const target = value?.targetUri
    ? { uri: value.targetUri, range: value.targetSelectionRange ?? value.targetRange }
    : value?.location
      ? value.location
      : value;
  const uri = target?.uri;
  const range = target?.range ?? target?.selectionRange;
  if (!uri || !range?.start) return null;
  const absolute = lspUriToPath(String(uri));
  const relative = rel(root, absolute);
  if (relative.startsWith("../") || isSensitivePath(relative)) return null;
  return {
    path: relative,
    line: Number(range.start.line ?? 0) + 1,
    column: Number(range.start.character ?? 0) + 1,
    endLine: Number(range.end?.line ?? range.start.line ?? 0) + 1,
    endColumn: Number(range.end?.character ?? range.start.character ?? 0) + 1,
    provider,
  };
}

function flattenLocations(root: string, value: any, provider: string) {
  const values = Array.isArray(value) ? value : value ? [value] : [];
  return values.map((item) => locationFromLsp(root, item, provider)).filter((item): item is SemanticLocation => !!item);
}

function symbolLocations(root: string, value: any, provider: string): SemanticLocation[] {
  const out: SemanticLocation[] = [];
  const visit = (item: any, fallbackUri?: string) => {
    if (!item) return;
    const uri = item.location?.uri ?? fallbackUri;
    const range = item.location?.range ?? item.selectionRange ?? item.range;
    if (uri && range?.start) {
      const loc = locationFromLsp(root, { uri, range }, provider);
      if (loc) out.push({ ...loc, name: String(item.name ?? ""), kind: String(item.kind ?? "") });
    }
    for (const child of Array.isArray(item.children) ? item.children : []) visit(child, uri);
  };
  for (const item of Array.isArray(value) ? value : value ? [value] : []) visit(item);
  return out;
}

function callHierarchyLocations(root: string, value: any, provider: string, direction: "incoming" | "outgoing") {
  const values = Array.isArray(value) ? value : value ? [value] : [];
  const out: SemanticLocation[] = [];
  for (const entry of values) {
    const item = direction === "incoming" ? entry?.from : entry?.to;
    const loc = locationFromLsp(root, item, provider);
    if (loc) out.push({ ...loc, name: item?.name ? String(item.name) : undefined, kind: "call-hierarchy" });
  }
  return out;
}

async function runCapture(command: string, args: string[], cwd: string, timeoutMs = 20_000) {
  return new Promise<{ stdout: string; stderr: string; code: number | null }>((resolve, reject) => {
    const child = spawn(command, args, { cwd, env: { ...process.env, PAGER: "cat", GIT_PAGER: "cat" }, stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "", stderr = "", done = false;
    const timer = setTimeout(() => { if (!done) child.kill("SIGTERM"); }, timeoutMs);
    child.stdout.on("data", (d) => { stdout = (stdout + d.toString()).slice(-2_000_000); });
    child.stderr.on("data", (d) => { stderr = (stderr + d.toString()).slice(-2_000_000); });
    child.on("error", reject);
    child.on("close", (code) => { done = true; clearTimeout(timer); resolve({ stdout, stderr, code }); });
  });
}

export class SemanticRouter {
  private ts: TypeScriptSemanticIndex;
  private clients = new Map<string, LspClient>();
  private brokenUntil = new Map<string, number>();
  private availability = new Map<string, { at: number; installed: boolean }>();
  private rootCache = new Map<string, string>();

  constructor(private root: string) {
    this.ts = new TypeScriptSemanticIndex(root);
  }

  invalidate() {
    this.ts.invalidate();
  }

  private specForFile(file: string) {
    const ext = path.extname(file).toLowerCase();
    return SPECS.find((spec) => spec.extensions.includes(ext)) ?? null;
  }

  private isTypeScriptLike(file?: string) {
    return !file || [".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"].includes(path.extname(file).toLowerCase());
  }

  private async installed(spec: LspServerSpec) {
    const cached = this.availability.get(spec.id);
    if (cached && Date.now() - cached.at < 30_000) return cached.installed;
    const installed = await commandExists(spec.command);
    this.availability.set(spec.id, { at: Date.now(), installed });
    return installed;
  }

  private async rootForFile(relativePath: string, spec: LspServerSpec) {
    const absolute = path.resolve(this.root, relativePath);
    const cacheKey = `${spec.id}:${path.dirname(absolute)}`;
    const cached = this.rootCache.get(cacheKey);
    if (cached) return cached;
    const resolved = await resolveLspRoot(this.root, absolute, spec);
    this.rootCache.set(cacheKey, resolved);
    return resolved;
  }

  private async clientFor(relativePath: string) {
    const spec = this.specForFile(relativePath);
    if (!spec || !(await this.installed(spec))) return null;
    const projectRoot = await this.rootForFile(relativePath, spec);
    const key = `${spec.id}:${projectRoot}`;
    if ((this.brokenUntil.get(key) ?? 0) > Date.now()) return null;
    let client = this.clients.get(key);
    if (!client) {
      client = new LspClient(projectRoot, spec);
      this.clients.set(key, client);
    }
    return { key, spec, client };
  }

  private async withClient<T>(relativePath: string, fn: (client: LspClient, spec: LspServerSpec) => Promise<T>): Promise<T | null> {
    const resolved = await this.clientFor(relativePath);
    if (!resolved) return null;
    try {
      return await fn(resolved.client, resolved.spec);
    } catch {
      this.brokenUntil.set(resolved.key, Date.now() + 30_000);
      this.clients.delete(resolved.key);
      await resolved.client.stop().catch(() => undefined);
      return null;
    }
  }

  async info() {
    const providers = await Promise.all(SPECS.map(async (spec) => ({
      id: spec.id,
      languages: spec.languages,
      installed: await this.installed(spec),
      activeRoots: [...this.clients.values()].filter((client) => client.spec.id === spec.id).map((client) => rel(this.root, client.root)),
    })));
    return {
      typescript: this.ts.info(),
      providers,
      activeClients: [...this.clients.values()].map((client) => ({ ...client.status(), root: rel(this.root, client.root) })),
      fallback: (await commandExists("rg")) ? "ripgrep" : "grep",
      routing: "nearest-project-root/polyglot",
    };
  }

  async identifierAt(relativePath: string, line: number, column: number) {
    const absolute = path.resolve(this.root, relativePath);
    const text = await fs.readFile(absolute, "utf8");
    const lines = text.split(/\r?\n/);
    const row = lines[Math.max(0, line - 1)] ?? "";
    const index = Math.max(0, Math.min(row.length, column - 1));
    const left = row.slice(0, index + 1).match(/[A-Za-z_$][\w$]*$/)?.[0] ?? "";
    const right = row.slice(index + 1).match(/^[\w$]*/)?.[0] ?? "";
    return `${left}${right}` || row.slice(index).match(/^[A-Za-z_$][\w$]*/)?.[0] || "";
  }

  async workspaceSymbols(query: string, limit = 200) {
    const out: SemanticLocation[] = this.ts.workspaceSymbols(query, limit).map((item) => ({ ...item, provider: "typescript" }));
    if (out.length >= limit) return out.slice(0, limit);

    for (const client of this.clients.values()) {
      if (out.length >= limit) break;
      const result = await client.workspaceSymbols(query).catch(() => []);
      for (const item of symbolLocations(this.root, result, client.spec.id)) {
        if (!out.some((existing) => existing.path === item.path && existing.line === item.line && existing.name === item.name)) out.push(item);
        if (out.length >= limit) break;
      }
    }

    if (query && out.length < limit) {
      const fallback = await this.textSearch(query, limit - out.length, true);
      for (const item of fallback) if (!out.some((existing) => existing.path === item.path && existing.line === item.line)) out.push(item);
    }
    return out.slice(0, limit);
  }

  async documentSymbols(relativePath: string, limit = 500) {
    if (this.isTypeScriptLike(relativePath)) {
      const ts = this.ts.workspaceSymbols("", 10_000).filter((item) => item.path === relativePath).slice(0, limit).map((item) => ({ ...item, provider: "typescript" }));
      if (ts.length) return ts;
    }
    const absolute = path.resolve(this.root, relativePath);
    const result = await this.withClient(relativePath, (client) => client.documentSymbols(absolute));
    if (result) return symbolLocations(this.root, result, this.specForFile(relativePath)?.id ?? "lsp").slice(0, limit);
    return this.textSearchInFile(relativePath, /\b(class|struct|interface|enum|trait|mixin|extension|def|fn|func|function|type|module)\s+([A-Za-z_$][\w$]*)/g, limit);
  }

  async definition(input: { path?: string; line?: number; column?: number; name?: string; limit?: number }) {
    const limit = input.limit ?? 100;
    if (input.path && input.line && input.column && !this.isTypeScriptLike(input.path)) {
      const provider = this.specForFile(input.path)?.id ?? "lsp";
      const value = await this.withClient(input.path, (client) => client.positionRequest("textDocument/definition", path.resolve(this.root, input.path!), input.line!, input.column!));
      const locations = value ? flattenLocations(this.root, value, provider) : [];
      if (locations.length) return locations.slice(0, limit);
    }
    const name = input.name || (input.path && input.line && input.column ? await this.identifierAt(input.path, input.line, input.column) : "");
    if (!name) return [];
    if (!input.path || this.isTypeScriptLike(input.path)) {
      const ts = this.ts.definitions(name, limit).map((item) => ({ ...item, provider: "typescript" }));
      if (ts.length) return ts;
    }
    return this.textSearch(name, limit, true);
  }

  async references(input: { path?: string; line?: number; column?: number; name?: string; limit?: number }) {
    const limit = input.limit ?? 500;
    if (input.path && input.line && input.column && !this.isTypeScriptLike(input.path)) {
      const provider = this.specForFile(input.path)?.id ?? "lsp";
      const value = await this.withClient(input.path, (client) => client.positionRequest("textDocument/references", path.resolve(this.root, input.path!), input.line!, input.column!, { context: { includeDeclaration: true } }));
      const locations = value ? flattenLocations(this.root, value, provider) : [];
      if (locations.length) return locations.slice(0, limit);
    }
    const name = input.name || (input.path && input.line && input.column ? await this.identifierAt(input.path, input.line, input.column) : "");
    if (!name) return [];
    if (!input.path || this.isTypeScriptLike(input.path)) {
      const ts = this.ts.references(name, limit).map((item) => ({ ...item, provider: "typescript" }));
      if (ts.length) return ts;
    }
    return this.textSearch(name, limit, false);
  }

  async implementations(input: { path: string; line: number; column: number; limit?: number }) {
    const provider = this.specForFile(input.path)?.id ?? "lsp";
    const value = await this.withClient(input.path, (client) => client.positionRequest("textDocument/implementation", path.resolve(this.root, input.path), input.line, input.column));
    const locations = value ? flattenLocations(this.root, value, provider) : [];
    if (locations.length) return locations.slice(0, input.limit ?? 200);
    return this.definition({ ...input, limit: input.limit });
  }

  async hover(input: { path: string; line: number; column: number }) {
    const provider = this.specForFile(input.path)?.id;
    if (provider) {
      const value = await this.withClient(input.path, (client) => client.positionRequest("textDocument/hover", path.resolve(this.root, input.path), input.line, input.column));
      if (value) return { provider, result: value };
    }
    const name = await this.identifierAt(input.path, input.line, input.column);
    return { provider: this.isTypeScriptLike(input.path) ? "typescript" : "text", name, definitions: await this.definition({ ...input, name, limit: 10 }) };
  }

  async diagnostics(relativePath?: string, limit = 500) {
    if (!relativePath) return this.ts.diagnostics(limit).map((item) => ({ ...item, provider: "typescript" }));
    if (this.isTypeScriptLike(relativePath)) {
      const ts = this.ts.diagnostics(limit).filter((item: any) => !item.path || item.path === relativePath).map((item) => ({ ...item, provider: "typescript" }));
      if (ts.length) return ts;
    }
    const provider = this.specForFile(relativePath)?.id;
    if (!provider) return [];
    const values = await this.withClient(relativePath, (client) => client.diagnostics(path.resolve(this.root, relativePath)));
    if (!values) return [];
    return values.slice(0, limit).map((diagnostic: any) => ({
      path: relativePath,
      line: Number(diagnostic.range?.start?.line ?? 0) + 1,
      column: Number(diagnostic.range?.start?.character ?? 0) + 1,
      severity: diagnostic.severity,
      code: diagnostic.code,
      message: diagnostic.message,
      provider,
    }));
  }

  async incomingCalls(input: { path: string; line: number; column: number; limit?: number }) {
    const provider = this.specForFile(input.path)?.id;
    if (!provider) return [];
    const value = await this.withClient(input.path, (client) => client.callHierarchy(path.resolve(this.root, input.path), input.line, input.column, "incoming"));
    return callHierarchyLocations(this.root, value ?? [], provider, "incoming").slice(0, input.limit ?? 200);
  }

  async outgoingCalls(input: { path: string; line: number; column: number; limit?: number }) {
    const provider = this.specForFile(input.path)?.id;
    if (!provider) return [];
    const value = await this.withClient(input.path, (client) => client.callHierarchy(path.resolve(this.root, input.path), input.line, input.column, "outgoing"));
    return callHierarchyLocations(this.root, value ?? [], provider, "outgoing").slice(0, input.limit ?? 200);
  }

  callers(name: string, limit = 300) {
    return this.ts.callers(name, limit).map((item) => ({ ...item, provider: "typescript" }));
  }

  callees(name: string, limit = 300) {
    return this.ts.callees(name, limit).map((item) => ({ ...item, provider: "typescript" }));
  }

  importGraph(limit = 2000) {
    return this.ts.importGraph(limit).map((item) => ({ ...item, provider: "typescript" }));
  }

  private async textSearch(query: string, limit: number, definitionLike: boolean): Promise<SemanticLocation[]> {
    if (!query) return [];
    const rg = await commandExists("rg");
    const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const pattern = definitionLike
      ? `\\b(class|struct|interface|enum|trait|mixin|extension|def|fn|func|function|type|const|let|var|final)\\s+${escaped}\\b|\\b${escaped}\\s*[:=]`
      : `\\b${escaped}\\b`;
    const result = rg
      ? await runCapture("rg", ["--line-number", "--column", "--no-heading", "--color", "never", "--hidden", "--glob", "!.git/**", "--glob", "!node_modules/**", pattern, "."], this.root).catch(() => null)
      : null;
    if (!result) return [];
    const out: SemanticLocation[] = [];
    for (const line of result.stdout.split("\n")) {
      const match = /^(.*?):(\d+):(\d+):/.exec(line);
      if (!match) continue;
      const relative = match[1].replace(/^\.\//, "");
      if (isSensitivePath(relative)) continue;
      out.push({ path: relative, line: Number(match[2]), column: Number(match[3]), name: query, provider: "ripgrep" });
      if (out.length >= limit) break;
    }
    return out;
  }

  private async textSearchInFile(relativePath: string, pattern: RegExp, limit: number) {
    const text = await fs.readFile(path.resolve(this.root, relativePath), "utf8");
    const out: SemanticLocation[] = [];
    for (const [index, line] of text.split(/\r?\n/).entries()) {
      pattern.lastIndex = 0;
      let match: RegExpExecArray | null;
      while ((match = pattern.exec(line))) {
        out.push({ path: relativePath, line: index + 1, column: match.index + 1, name: match[2], provider: "text-structure" });
        if (out.length >= limit) return out;
        if (!pattern.global) break;
      }
    }
    return out;
  }

  async shutdown() {
    await Promise.all([...this.clients.values()].map((client) => client.stop().catch(() => undefined)));
    this.clients.clear();
  }
}
