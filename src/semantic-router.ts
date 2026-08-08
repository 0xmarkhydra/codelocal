import path from "node:path";
import { fileURLToPath } from "node:url";
import { promises as fs } from "node:fs";
import { spawn } from "node:child_process";
import { TypeScriptSemanticIndex } from "./semantic.js";
import { LspClient, commandExists, type LspServerSpec } from "./lsp.js";
import { isSensitivePath } from "./security-policy.js";

export type SemanticLocation = { path: string; line: number; column: number; endLine?: number; endColumn?: number; name?: string; kind?: string; provider?: string };

const SPECS: LspServerSpec[] = [
  { id: "pyright", command: "pyright-langserver", args: ["--stdio"], languages: ["python"], extensions: [".py"] },
  { id: "rust-analyzer", command: "rust-analyzer", args: [], languages: ["rust"], extensions: [".rs"] },
  { id: "gopls", command: "gopls", args: [], languages: ["go"], extensions: [".go"] },
  { id: "clangd", command: "clangd", args: ["--background-index"], languages: ["c", "cpp"], extensions: [".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh"] },
  { id: "jdtls", command: "jdtls", args: [], languages: ["java"], extensions: [".java"] },
  { id: "kotlin-language-server", command: "kotlin-language-server", args: [], languages: ["kotlin"], extensions: [".kt", ".kts"] },
  { id: "lua-language-server", command: "lua-language-server", args: [], languages: ["lua"], extensions: [".lua"] },
  { id: "sourcekit-lsp", command: "sourcekit-lsp", args: [], languages: ["swift"], extensions: [".swift"] },
  { id: "zls", command: "zls", args: [], languages: ["zig"], extensions: [".zig"] },
];

function rel(root: string, file: string) {
  return path.relative(root, file).split(path.sep).join("/") || ".";
}

function lspUriToPath(uri: string) {
  try { return fileURLToPath(uri); } catch { return uri; }
}

function locationFromLsp(root: string, value: any, provider: string): SemanticLocation | null {
  const target = value?.targetUri ? { uri: value.targetUri, range: value.targetSelectionRange ?? value.targetRange } : value;
  const uri = target?.uri;
  const range = target?.range;
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

  constructor(private root: string) {
    this.ts = new TypeScriptSemanticIndex(root);
    for (const spec of SPECS) this.clients.set(spec.id, new LspClient(root, spec));
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

  async info() {
    const providers = await Promise.all(SPECS.map(async (spec) => ({ id: spec.id, languages: spec.languages, installed: await commandExists(spec.command) })));
    return {
      typescript: this.ts.info(),
      providers,
      fallback: (await commandExists("rg")) ? "ripgrep" : "grep",
      routing: "file-extension/polyglot",
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
    const primary = this.ts.workspaceSymbols(query, limit).map((x) => ({ ...x, provider: "typescript" }));
    if (primary.length >= limit || !query) return primary.slice(0, limit);
    const fallback = await this.textSearch(query, limit - primary.length, true);
    return [...primary, ...fallback];
  }

  async documentSymbols(relativePath: string, limit = 500) {
    const absolute = path.resolve(this.root, relativePath);
    if (this.isTypeScriptLike(relativePath)) {
      return this.ts.workspaceSymbols("", 5000).filter((x) => x.path === relativePath).slice(0, limit).map((x) => ({ ...x, provider: "typescript" }));
    }
    const spec = this.specForFile(relativePath);
    if (spec && await commandExists(spec.command)) {
      const result = await this.clients.get(spec.id)!.documentSymbols(absolute);
      return symbolLocations(this.root, result, spec.id).slice(0, limit);
    }
    return this.textSearchInFile(relativePath, /\b(class|struct|interface|enum|trait|def|fn|func|function|type|module)\s+([A-Za-z_$][\w$]*)/g, limit);
  }

  async definition(input: { path?: string; line?: number; column?: number; name?: string; limit?: number }) {
    const limit = input.limit ?? 100;
    if (input.path && input.line && input.column && !this.isTypeScriptLike(input.path)) {
      const spec = this.specForFile(input.path);
      if (spec && await commandExists(spec.command)) {
        const result = await this.clients.get(spec.id)!.positionRequest("textDocument/definition", path.resolve(this.root, input.path), input.line, input.column);
        return flattenLocations(this.root, result, spec.id).slice(0, limit);
      }
    }
    const name = input.name || (input.path && input.line && input.column ? await this.identifierAt(input.path, input.line, input.column) : "");
    if (!name) return [];
    if (!input.path || this.isTypeScriptLike(input.path)) {
      const ts = this.ts.definitions(name, limit).map((x) => ({ ...x, provider: "typescript" }));
      if (ts.length) return ts;
    }
    return this.textSearch(name, limit, true);
  }

  async references(input: { path?: string; line?: number; column?: number; name?: string; limit?: number }) {
    const limit = input.limit ?? 500;
    if (input.path && input.line && input.column && !this.isTypeScriptLike(input.path)) {
      const spec = this.specForFile(input.path);
      if (spec && await commandExists(spec.command)) {
        const result = await this.clients.get(spec.id)!.positionRequest("textDocument/references", path.resolve(this.root, input.path), input.line, input.column, { context: { includeDeclaration: true } });
        return flattenLocations(this.root, result, spec.id).slice(0, limit);
      }
    }
    const name = input.name || (input.path && input.line && input.column ? await this.identifierAt(input.path, input.line, input.column) : "");
    if (!name) return [];
    if (!input.path || this.isTypeScriptLike(input.path)) {
      const ts = this.ts.references(name, limit).map((x) => ({ ...x, provider: "typescript" }));
      if (ts.length) return ts;
    }
    return this.textSearch(name, limit, false);
  }

  async implementations(input: { path: string; line: number; column: number; limit?: number }) {
    const spec = this.specForFile(input.path);
    if (spec && await commandExists(spec.command)) {
      const result = await this.clients.get(spec.id)!.positionRequest("textDocument/implementation", path.resolve(this.root, input.path), input.line, input.column);
      return flattenLocations(this.root, result, spec.id).slice(0, input.limit ?? 200);
    }
    return this.definition({ ...input, limit: input.limit });
  }

  async hover(input: { path: string; line: number; column: number }) {
    const spec = this.specForFile(input.path);
    if (spec && await commandExists(spec.command)) {
      return { provider: spec.id, result: await this.clients.get(spec.id)!.positionRequest("textDocument/hover", path.resolve(this.root, input.path), input.line, input.column) };
    }
    const name = await this.identifierAt(input.path, input.line, input.column);
    return { provider: this.isTypeScriptLike(input.path) ? "typescript" : "text", name, definitions: await this.definition({ ...input, name, limit: 10 }) };
  }

  async diagnostics(relativePath?: string, limit = 500) {
    if (!relativePath) return this.ts.diagnostics(limit).map((x) => ({ ...x, provider: "typescript" }));
    if (this.isTypeScriptLike(relativePath)) return this.ts.diagnostics(limit).filter((x: any) => !x.path || x.path === relativePath).map((x) => ({ ...x, provider: "typescript" }));
    const spec = this.specForFile(relativePath);
    if (spec && await commandExists(spec.command)) {
      const values = await this.clients.get(spec.id)!.diagnostics(path.resolve(this.root, relativePath));
      return values.slice(0, limit).map((d: any) => ({
        path: relativePath,
        line: Number(d.range?.start?.line ?? 0) + 1,
        column: Number(d.range?.start?.character ?? 0) + 1,
        severity: d.severity,
        code: d.code,
        message: d.message,
        provider: spec.id,
      }));
    }
    return [];
  }

  callers(name: string, limit = 300) {
    return this.ts.callers(name, limit).map((x) => ({ ...x, provider: "typescript" }));
  }

  callees(name: string, limit = 300) {
    return this.ts.callees(name, limit).map((x) => ({ ...x, provider: "typescript" }));
  }

  importGraph(limit = 2000) {
    return this.ts.importGraph(limit).map((x) => ({ ...x, provider: "typescript" }));
  }

  private async textSearch(query: string, limit: number, definitionLike: boolean): Promise<SemanticLocation[]> {
    if (!query) return [];
    const rg = await commandExists("rg");
    const pattern = definitionLike ? `\\b(class|struct|interface|enum|trait|def|fn|func|function|type|const|let|var)\\s+${query}\\b|\\b${query}\\s*[:=]` : `\\b${query}\\b`;
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
  }
}
