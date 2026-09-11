import ts from "typescript";
import path from "node:path";
import { readdirSync } from "node:fs";

export type Location = { path: string; line: number; column: number; kind?: string; name?: string; text?: string };

type TsProject = {
  configPath: string;
  program: ts.Program;
  checker: ts.TypeChecker;
};

const SKIP_DIRS = new Set([".git", "node_modules", ".next", "dist", "build", "target", ".venv", "venv", "coverage", ".cache", ".turbo", ".dart_tool", "Pods", "DerivedData"]);

function rel(root: string, file: string) {
  return path.relative(path.resolve(root), path.resolve(file)).split(path.sep).join("/") || ".";
}

function inside(root: string, file: string) {
  const relative = path.relative(path.resolve(root), path.resolve(file));
  return relative === "" || (!relative.startsWith(`..${path.sep}`) && relative !== ".." && !path.isAbsolute(relative));
}

function isNodeModules(root: string, file: string) {
  const relative = rel(root, file);
  return relative === "node_modules" || relative.startsWith("node_modules/") || relative.includes("/node_modules/");
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

function discoverConfigs(root: string, maxDepth: number, maxProjects: number) {
  const out: string[] = [];
  const walk = (dir: string, depth: number) => {
    if (depth > maxDepth || out.length >= maxProjects) return;
    let entries: ReturnType<typeof readdirSync>;
    try { entries = readdirSync(dir, { withFileTypes: true }) as any; } catch { return; }
    const names = new Set((entries as any[]).map((entry) => entry.name));
    for (const candidate of ["tsconfig.json", "jsconfig.json"]) {
      if (names.has(candidate)) {
        out.push(path.join(dir, candidate));
        if (out.length >= maxProjects) return;
      }
    }
    for (const entry of entries as any[]) {
      if (!entry.isDirectory() || SKIP_DIRS.has(entry.name)) continue;
      walk(path.join(dir, entry.name), depth + 1);
      if (out.length >= maxProjects) return;
    }
  };
  walk(root, 0);
  return out.sort((a, b) => {
    const da = rel(root, a).split("/").length;
    const db = rel(root, b).split("/").length;
    return da - db || a.localeCompare(b);
  });
}

export class TypeScriptSemanticIndex {
  private projects: TsProject[] = [];
  private configPaths: string[] = [];
  private builtAt = 0;
  private attempted = false;

  constructor(private root: string) {}

  invalidate() {
    this.projects = [];
    this.configPaths = [];
    this.builtAt = 0;
    this.attempted = false;
  }

  private build() {
    if (this.attempted) return;
    this.attempted = true;

    const maxProjects = Math.max(1, Number(process.env.CODELOCAL_TS_MAX_PROJECTS ?? 20) || 20);
    const maxDepth = Math.max(1, Number(process.env.CODELOCAL_TS_CONFIG_DEPTH ?? 6) || 6);
    const maxFilesPerProject = Math.max(100, Number(process.env.CODELOCAL_TS_MAX_FILES_PER_PROJECT ?? 4_000) || 4_000);
    const maxTotalFiles = Math.max(maxFilesPerProject, Number(process.env.CODELOCAL_TS_MAX_TOTAL_FILES ?? 12_000) || 12_000);
    const configs = discoverConfigs(this.root, maxDepth, maxProjects);
    let remaining = maxTotalFiles;

    for (const config of configs) {
      if (remaining <= 0) break;
      const read = ts.readConfigFile(config, ts.sys.readFile);
      if (read.error) continue;
      const parsed = ts.parseJsonConfigFileContent(read.config ?? {}, ts.sys, path.dirname(config));
      const rootNames = parsed.fileNames.slice(0, Math.min(maxFilesPerProject, remaining));
      if (!rootNames.length) continue;
      remaining -= rootNames.length;
      const options: ts.CompilerOptions = { ...parsed.options, noEmit: true, skipLibCheck: true };
      try {
        const program = ts.createProgram({ rootNames, options });
        this.projects.push({ configPath: config, program, checker: program.getTypeChecker() });
        this.configPaths.push(config);
      } catch {}
    }

    this.builtAt = Date.now();
  }

  info() {
    this.build();
    return {
      available: this.projects.length > 0,
      configured: this.configPaths.length > 0,
      mode: this.projects.length > 1 ? "typescript-multi-project" : this.projects.length === 1 ? "typescript-program" : "fallback",
      configPath: this.configPaths[0] ? rel(this.root, this.configPaths[0]) : null,
      configPaths: this.configPaths.map((config) => rel(this.root, config)),
      projectCount: this.projects.length,
      sourceFiles: this.projectSources().length,
      builtAt: this.builtAt,
    };
  }

  private projectSources() {
    this.build();
    const unique = new Map<string, ts.SourceFile>();
    for (const project of this.projects) {
      for (const source of project.program.getSourceFiles()) {
        if (!inside(this.root, source.fileName)) continue;
        if (isNodeModules(this.root, source.fileName)) continue;
        unique.set(path.resolve(source.fileName), source);
      }
    }
    return [...unique.values()];
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
      const visit = (node: ts.Node, insideOwner = false) => {
        const n = nodeName(node);
        const matchesOwner = !!n && ts.isIdentifier(n) && n.text === name && (ts.isFunctionDeclaration(node) || ts.isMethodDeclaration(node) || ts.isFunctionExpression(node));
        const nextInside = insideOwner || matchesOwner;
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
    const out: Array<{ path?: string; line?: number; column?: number; message: string; code: number; category: string }> = [];
    const seen = new Set<string>();
    for (const project of this.projects) {
      for (const d of ts.getPreEmitDiagnostics(project.program)) {
        const message = ts.flattenDiagnosticMessageText(d.messageText, "\n");
        const file = d.file?.fileName;
        const key = `${file ?? ""}:${d.start ?? -1}:${d.code}:${message}`;
        if (seen.has(key)) continue;
        seen.add(key);
        if (!d.file || d.start == null) out.push({ message, code: d.code, category: ts.DiagnosticCategory[d.category] });
        else {
          const p = d.file.getLineAndCharacterOfPosition(d.start);
          out.push({ path: rel(this.root, d.file.fileName), line: p.line + 1, column: p.character + 1, message, code: d.code, category: ts.DiagnosticCategory[d.category] });
        }
        if (out.length >= limit) return out;
      }
    }
    return out;
  }
}
