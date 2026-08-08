import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { pathToFileURL } from "node:url";
import { promises as fs } from "node:fs";
import path from "node:path";

export type LspServerSpec = {
  id: string;
  command: string;
  args: string[];
  languages: string[];
  extensions: string[];
  initializationOptions?: unknown;
};

type Pending = { resolve: (value: any) => void; reject: (error: Error) => void; timer: NodeJS.Timeout };

function uri(file: string) {
  return pathToFileURL(file).toString();
}

function languageId(file: string) {
  const ext = path.extname(file).toLowerCase();
  const map: Record<string, string> = {
    ".ts": "typescript", ".tsx": "typescriptreact", ".js": "javascript", ".jsx": "javascriptreact",
    ".py": "python", ".rs": "rust", ".go": "go", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp", ".hpp": "cpp",
    ".java": "java", ".kt": "kotlin", ".kts": "kotlin", ".cs": "csharp", ".php": "php", ".rb": "ruby", ".lua": "lua",
    ".swift": "swift", ".dart": "dart", ".ex": "elixir", ".exs": "elixir", ".zig": "zig", ".sol": "solidity",
  };
  return map[ext] ?? (ext.replace(/^\./, "") || "plaintext");
}

export async function commandExists(command: string) {
  const pathEntries = (process.env.PATH ?? "").split(path.delimiter);
  const suffixes = process.platform === "win32" ? ["", ".exe", ".cmd", ".bat"] : [""];
  for (const dir of pathEntries) {
    for (const suffix of suffixes) {
      try {
        await fs.access(path.join(dir, `${command}${suffix}`));
        return true;
      } catch {}
    }
  }
  return false;
}

export class LspClient {
  private child: ChildProcessWithoutNullStreams | null = null;
  private sequence = 1;
  private buffer = Buffer.alloc(0);
  private pending = new Map<number, Pending>();
  private opened = new Map<string, number>();
  private publishedDiagnostics = new Map<string, any[]>();
  private initialized = false;

  constructor(private root: string, public readonly spec: LspServerSpec) {}

  async available() {
    return commandExists(this.spec.command);
  }

  async start() {
    if (this.initialized && this.child) return;
    if (!(await this.available())) throw new Error(`${this.spec.id} is not installed.`);
    this.child = spawn(this.spec.command, this.spec.args, {
      cwd: this.root,
      env: { ...process.env },
      stdio: "pipe",
    }) as ChildProcessWithoutNullStreams;
    this.child.stdout.on("data", (chunk: Buffer) => this.onData(chunk));
    this.child.stderr.on("data", () => undefined);
    this.child.on("error", (error) => this.failAll(error));
    this.child.on("close", () => this.failAll(new Error(`${this.spec.id} exited.`)));

    const rootUri = uri(this.root);
    await this.request("initialize", {
      processId: process.pid,
      clientInfo: { name: "CodeLocal", version: "1.0" },
      rootUri,
      workspaceFolders: [{ uri: rootUri, name: path.basename(this.root) }],
      capabilities: {
        workspace: { symbol: {}, workspaceFolders: true },
        textDocument: {
          synchronization: { didSave: true, dynamicRegistration: false },
          definition: {}, references: {}, implementation: {}, hover: {}, documentSymbol: {}, publishDiagnostics: {},
        },
      },
      initializationOptions: this.spec.initializationOptions,
    }, 20_000);
    this.notify("initialized", {});
    this.initialized = true;
  }

  private failAll(error: Error) {
    for (const [id, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(error);
      this.pending.delete(id);
    }
    this.initialized = false;
    this.child = null;
  }

  private send(payload: unknown) {
    if (!this.child) throw new Error(`${this.spec.id} is not running.`);
    const json = JSON.stringify(payload);
    this.child.stdin.write(`Content-Length: ${Buffer.byteLength(json, "utf8")}\r\n\r\n${json}`);
  }

  request(method: string, params: unknown, timeoutMs = 15_000) {
    const id = this.sequence++;
    const promise = new Promise<any>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`${this.spec.id} LSP request timed out: ${method}`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
    });
    this.send({ jsonrpc: "2.0", id, method, params });
    return promise;
  }

  notify(method: string, params: unknown) {
    this.send({ jsonrpc: "2.0", method, params });
  }

  private onData(chunk: Buffer) {
    this.buffer = Buffer.concat([this.buffer, chunk]);
    while (true) {
      const headerEnd = this.buffer.indexOf("\r\n\r\n");
      if (headerEnd < 0) return;
      const header = this.buffer.subarray(0, headerEnd).toString("ascii");
      const match = /Content-Length:\s*(\d+)/i.exec(header);
      if (!match) {
        this.buffer = this.buffer.subarray(headerEnd + 4);
        continue;
      }
      const length = Number(match[1]);
      const bodyStart = headerEnd + 4;
      if (this.buffer.length < bodyStart + length) return;
      const body = this.buffer.subarray(bodyStart, bodyStart + length).toString("utf8");
      this.buffer = this.buffer.subarray(bodyStart + length);
      try { this.onMessage(JSON.parse(body)); } catch {}
    }
  }

  private onMessage(message: any) {
    if (typeof message?.id === "number" && this.pending.has(message.id)) {
      const pending = this.pending.get(message.id)!;
      this.pending.delete(message.id);
      clearTimeout(pending.timer);
      if (message.error) pending.reject(new Error(message.error.message ?? "LSP error"));
      else pending.resolve(message.result);
      return;
    }
    if (message?.method === "textDocument/publishDiagnostics") {
      this.publishedDiagnostics.set(String(message.params?.uri ?? ""), Array.isArray(message.params?.diagnostics) ? message.params.diagnostics : []);
    }
  }

  async openDocument(file: string) {
    await this.start();
    const absolute = path.resolve(file);
    const text = await fs.readFile(absolute, "utf8");
    const version = (this.opened.get(absolute) ?? 0) + 1;
    this.opened.set(absolute, version);
    const params = { textDocument: { uri: uri(absolute), languageId: languageId(absolute), version, text } };
    if (version === 1) this.notify("textDocument/didOpen", params);
    else this.notify("textDocument/didChange", { textDocument: { uri: uri(absolute), version }, contentChanges: [{ text }] });
    return { absolute, version };
  }

  async positionRequest(method: string, file: string, line: number, column: number, extra: Record<string, unknown> = {}) {
    await this.openDocument(file);
    return this.request(method, {
      textDocument: { uri: uri(path.resolve(file)) },
      position: { line: Math.max(0, line - 1), character: Math.max(0, column - 1) },
      ...extra,
    });
  }

  async workspaceSymbols(query: string) {
    await this.start();
    return this.request("workspace/symbol", { query });
  }

  async documentSymbols(file: string) {
    await this.openDocument(file);
    return this.request("textDocument/documentSymbol", { textDocument: { uri: uri(path.resolve(file)) } });
  }

  async diagnostics(file: string, waitMs = 350) {
    await this.openDocument(file);
    await new Promise((resolve) => setTimeout(resolve, waitMs));
    return this.publishedDiagnostics.get(uri(path.resolve(file))) ?? [];
  }

  async stop() {
    if (!this.child) return;
    try { await this.request("shutdown", null, 2000); } catch {}
    try { this.notify("exit", null); } catch {}
    this.child.kill("SIGTERM");
    this.child = null;
    this.initialized = false;
  }
}
