import ts from "typescript";
import path from "node:path";

export type Location = { path: string; line: number; column: number; kind?: string; name?: string; text?: string };

function rel(root: string, file: string) {
  return path.relative(root, file).split(path.sep).join("/") || ".";
}

function pos(source: ts.SourceFile, node: ts.Node) {
  const p = source.getLineAndCharacterOfPosition(node.getStart(source));
  return { line: p.line + 1, column: p.character + 1 };
}

function nodeName(node: ts.Node): ts.Node | undefined {
  const anyNode = node as ts.Node & { name?: ts.Node };
  return anyNode.name;
}

function kindName(node: ts.Node) {
  return ts.SyntaxKind[node.kind] ?? "Unknown";
}

export class TypeScriptSemanticIndex {
  private program: ts.Program | null = null;
  private checker: ts.TypeChecker | null = null;
  private configPath: string | null = null;
  private builtAt = 0;
  private attempted = false;
  constructor(private root: string) {}

  invalidate() {
    this.program = null;
    this.checker = null;
    this.configPath = null;
    this.builtAt = 0;
    this.attempted = false;
  }

  private build() {
    if (this.attempted) return;
    this.attempted = true;

    // Never synthesize a giant TypeScript Program for an arbitrary/generic root.
    // In polyglot monorepos this can include generated JS, vendored sources and
    // nested applications and easily consume multiple GB of heap. A configured
    // TS/JS project gets full compiler semantics; generic roots use LSP/ripgrep
    // fallback from SemanticRouter until a scoped project root is selected.
    const config = ts.findConfigFile(this.root, ts.sys.fileExists, "tsconfig.json") || ts.findConfigFile(this.root, ts.sys.fileExists, "jsconfig.json");
    if (!config) {
      this.configPath = null;
      this.builtAt = Date.now();
      return;
    }

    this.configPath = config;
    const read = ts.readConfigFile(config, ts.sys.readFile);
    if (read.error) {
      this.builtAt = Date.now();
      return;
    }
    const parsed = ts.parseJsonConfigFileContent(read.config ?? {}, ts.sys, path.dirname(config));
    const maxFiles = Math.max(100, Number(process.env.CODELOCAL_TS_MAX_FILES ?? 10_000) || 10_000);
    const rootNames = parsed.fileNames.slice(0, maxFiles);
    const options: ts.CompilerOptions = { ...parsed.options, noEmit: true, skipLibCheck: true };
    this.program = ts.createProgram({ rootNames, options });
    this.checker = this.program.getTypeChecker();
    this.builtAt = Date.now();
  }

  info() {
    this.build();
    return {
      available: !!this.program,
      configured: !!this.configPath,
      mode: this.program ? "typescript-program" : "fallback",
      configPath: this.configPath ? rel(this.root, this.configPath) : null,
      sourceFiles: this.program ? this.projectSources().length : 0,
      builtAt: this.builtAt,
    };
  }

  private projectSources() {
    this.build();
    return (this.program?.getSourceFiles() ?? []).filter((f) => f.fileName.startsWith(this.root + path.sep) && !f.fileName.includes(`${path.sep}node_modules${path.sep}`));
  }

  workspaceSymbols(query = "", limit = 200): Location[] {
    const q = query.toLowerCase();
    const out: Location[] = [];
    for (const source of this.projectSources()) {
      const visit = (node: ts.Node) => {
        const name = nodeName(node);
        if (name && (ts.isIdentifier(name) || ts.isStringLiteral(name))) {
          const n = name.text;
          if (!q || n.toLowerCase().includes(q)) {
            out.push({ path: rel(this.root, source.fileName), ...pos(source, name), kind: kindName(node), name: n });
            if (out.length >= limit) return;
          }
        }
        if (out.length < limit) ts.forEachChild(node, visit);
      };
      visit(source);
      if (out.length >= limit) break;
    }
    return out;
  }

  definitions(name: string, limit = 100) {
    return this.workspaceSymbols(name, limit).filter((x) => x.name === name);
  }

  references(name: string, limit = 500): Location[] {
    const out: Location[] = [];
    for (const source of this.projectSources()) {
      const visit = (node: ts.Node) => {
        if (ts.isIdentifier(node) && node.text === name) {
          out.push({ path: rel(this.root, source.fileName), ...pos(source, node), kind: kindName(node.parent), name });
        }
        if (out.length < limit) ts.forEachChild(node, visit);
      };
      visit(source);
      if (out.length >= limit) break;
    }
    return out;
  }

  callers(name: string, limit = 300): Location[] {
    const out: Location[] = [];
    for (const source of this.projectSources()) {
      const stack: ts.Node[] = [];
      const visit = (node: ts.Node) => {
        stack.push(node);
        if (ts.isCallExpression(node)) {
          const expression = node.expression;
          const called = ts.isIdentifier(expression) ? expression.text : ts.isPropertyAccessExpression(expression) ? expression.name.text : null;
          if (called === name) {
            const owner: ts.Node | undefined = stack.slice(0, -1).reverse().find((n) => ts.isFunctionDeclaration(n) || ts.isMethodDeclaration(n) || ts.isArrowFunction(n) || ts.isFunctionExpression(n));
            const ownerName = owner ? nodeName(owner) : undefined;
            out.push({ path: rel(this.root, source.fileName), ...pos(source, node), kind: "CallExpression", name: ownerName && ts.isIdentifier(ownerName) ? ownerName.text : undefined });
          }
        }
        if (out.length < limit) ts.forEachChild(node, visit);
        stack.pop();
      };
      visit(source);
      if (out.length >= limit) break;
    }
    return out;
  }

  callees(name: string, limit = 300): Location[] {
    const out: Location[] = [];
    for (const source of this.projectSources()) {
      const visit = (node: ts.Node, inside = false) => {
        const n = nodeName(node);
        const matchesOwner = !!n && ts.isIdentifier(n) && n.text === name && (ts.isFunctionDeclaration(node) || ts.isMethodDeclaration(node) || ts.isFunctionExpression(node));
        const nextInside = inside || matchesOwner;
        if (nextInside && ts.isCallExpression(node)) {
          const expression = node.expression;
          const called = ts.isIdentifier(expression) ? expression.text : ts.isPropertyAccessExpression(expression) ? expression.name.text : null;
          if (called) out.push({ path: rel(this.root, source.fileName), ...pos(source, node), kind: "CallExpression", name: called });
        }
        if (out.length < limit) ts.forEachChild(node, (child) => visit(child, nextInside));
      };
      visit(source, false);
      if (out.length >= limit) break;
    }
    return out;
  }

  importGraph(limit = 2000) {
    const edges: Array<{ from: string; to: string; kind: string }> = [];
    for (const source of this.projectSources()) {
      for (const statement of source.statements) {
        if (ts.isImportDeclaration(statement) || ts.isExportDeclaration(statement)) {
          const spec = statement.moduleSpecifier;
          if (spec && ts.isStringLiteral(spec)) edges.push({ from: rel(this.root, source.fileName), to: spec.text, kind: ts.isImportDeclaration(statement) ? "import" : "export" });
        }
      }
      if (edges.length >= limit) break;
    }
    return edges.slice(0, limit);
  }

  diagnostics(limit = 500) {
    this.build();
    if (!this.program) return [];
    const diags = ts.getPreEmitDiagnostics(this.program).slice(0, limit);
    return diags.map((d) => {
      const message = ts.flattenDiagnosticMessageText(d.messageText, "\n");
      if (!d.file || d.start == null) return { message, code: d.code, category: ts.DiagnosticCategory[d.category] };
      const p = d.file.getLineAndCharacterOfPosition(d.start);
      return { path: rel(this.root, d.file.fileName), line: p.line + 1, column: p.character + 1, message, code: d.code, category: ts.DiagnosticCategory[d.category] };
    });
  }
}
