import { promises as fs } from "node:fs";
import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { spawn } from "node:child_process";
import { isSensitivePath } from "./security-policy.js";

export type TextEdit = {
  startOffset?: number;
  endOffset?: number;
  startLine?: number;
  startColumn?: number;
  endLine?: number;
  endColumn?: number;
  replacement: string;
};

export type FileEdit = {
  path: string;
  expectedHash?: string;
  edits: TextEdit[];
};

function sha256(value: Buffer | string) {
  return createHash("sha256").update(value).digest("hex");
}

function offsetAt(text: string, line: number, column: number) {
  const lines = text.split(/(?<=\n)/);
  const row = Math.max(1, line);
  const col = Math.max(1, column);
  let offset = 0;
  for (let i = 0; i < row - 1 && i < lines.length; i++) offset += lines[i].length;
  return Math.min(text.length, offset + col - 1);
}

async function executable(file: string) {
  return fs.access(file).then(() => true).catch(() => false);
}

async function run(command: string, args: string[], cwd: string, timeoutMs = 120_000) {
  return new Promise<{ code: number | null; stdout: string; stderr: string }>((resolve, reject) => {
    const child = spawn(command, args, { cwd, env: { ...process.env }, stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "", stderr = "", done = false;
    const timer = setTimeout(() => { if (!done) child.kill("SIGTERM"); }, timeoutMs);
    child.stdout.on("data", (d) => { stdout = (stdout + d.toString()).slice(-1_000_000); });
    child.stderr.on("data", (d) => { stderr = (stderr + d.toString()).slice(-1_000_000); });
    child.on("error", reject);
    child.on("close", (code) => { done = true; clearTimeout(timer); resolve({ code, stdout, stderr }); });
  });
}

export class EditingEngine {
  constructor(private root: string) {}

  private lexical(relativePath: string) {
    if (path.isAbsolute(relativePath)) throw new Error("Absolute paths are not allowed.");
    if (isSensitivePath(relativePath)) throw new Error(`Access blocked by sensitive-path policy: ${relativePath}`);
    const candidate = path.resolve(this.root, relativePath);
    const prefix = this.root.endsWith(path.sep) ? this.root : this.root + path.sep;
    if (candidate !== this.root && !candidate.startsWith(prefix)) throw new Error("Path escapes PROJECT_ROOT.");
    return candidate;
  }

  private async existing(relativePath: string) {
    const candidate = this.lexical(relativePath);
    const real = await fs.realpath(candidate);
    const realRoot = await fs.realpath(this.root);
    const prefix = realRoot.endsWith(path.sep) ? realRoot : realRoot + path.sep;
    if (real !== realRoot && !real.startsWith(prefix)) throw new Error("Resolved path escapes PROJECT_ROOT.");
    return real;
  }

  async fileHash(relativePath: string) {
    return sha256(await fs.readFile(await this.existing(relativePath)));
  }

  private applyOne(text: string, edits: TextEdit[]) {
    const normalized = edits.map((edit) => {
      const start = edit.startOffset ?? (edit.startLine && edit.startColumn ? offsetAt(text, edit.startLine, edit.startColumn) : NaN);
      const end = edit.endOffset ?? (edit.endLine && edit.endColumn ? offsetAt(text, edit.endLine, edit.endColumn) : start);
      if (!Number.isFinite(start) || !Number.isFinite(end) || start < 0 || end < start || end > text.length) throw new Error("Invalid edit range.");
      return { start, end, replacement: edit.replacement };
    }).sort((a, b) => b.start - a.start || b.end - a.end);

    let previousStart = text.length + 1;
    for (const edit of normalized) {
      if (edit.end > previousStart) throw new Error("Overlapping edits are not allowed.");
      previousStart = edit.start;
    }

    let updated = text;
    for (const edit of normalized) updated = updated.slice(0, edit.start) + edit.replacement + updated.slice(edit.end);
    return updated;
  }

  async applyEdits(files: FileEdit[]) {
    if (!files.length) throw new Error("No edits provided.");
    const prepared: Array<{ relative: string; absolute: string; original: Buffer; updated: Buffer; originalHash: string; updatedHash: string; temp: string; backup: string }> = [];
    const seen = new Set<string>();

    for (const fileEdit of files) {
      if (seen.has(fileEdit.path)) throw new Error(`Duplicate edit target: ${fileEdit.path}`);
      seen.add(fileEdit.path);
      const absolute = await this.existing(fileEdit.path);
      const original = await fs.readFile(absolute);
      const originalHash = sha256(original);
      if (fileEdit.expectedHash && fileEdit.expectedHash !== originalHash) throw new Error(`File changed since read. Expected ${fileEdit.expectedHash}, got ${originalHash}.`);
      const text = original.toString("utf8");
      if (text.includes("\u0000")) throw new Error(`Binary file cannot be structurally edited: ${fileEdit.path}`);
      const updated = Buffer.from(this.applyOne(text, fileEdit.edits), "utf8");
      prepared.push({
        relative: fileEdit.path,
        absolute,
        original,
        updated,
        originalHash,
        updatedHash: sha256(updated),
        temp: path.join(path.dirname(absolute), `.${path.basename(absolute)}.${randomUUID()}.codelocal.tmp`),
        backup: path.join(path.dirname(absolute), `.${path.basename(absolute)}.${randomUUID()}.codelocal.bak`),
      });
    }

    for (const item of prepared) {
      await fs.writeFile(item.temp, item.updated, { mode: (await fs.stat(item.absolute)).mode });
    }

    const committed: typeof prepared = [];
    try {
      for (const item of prepared) {
        await fs.rename(item.absolute, item.backup);
        await fs.rename(item.temp, item.absolute);
        committed.push(item);
      }
      for (const item of committed) await fs.unlink(item.backup).catch(() => undefined);
    } catch (error) {
      for (const item of committed.reverse()) {
        await fs.unlink(item.absolute).catch(() => undefined);
        await fs.rename(item.backup, item.absolute).catch(() => undefined);
      }
      throw error;
    } finally {
      for (const item of prepared) {
        await fs.unlink(item.temp).catch(() => undefined);
        if (!committed.includes(item)) await fs.unlink(item.backup).catch(() => undefined);
      }
    }

    return {
      changed: prepared.map((item) => ({ path: item.relative, beforeHash: item.originalHash, afterHash: item.updatedHash, bytes: item.updated.length })),
      atomicValidation: true,
      rollbackOnFailure: true,
    };
  }

  async formatChangedFiles(relativePaths: string[]) {
    const paths = [...new Set(relativePaths)].filter(Boolean);
    if (!paths.length) return { formatted: [], skipped: [] };
    for (const p of paths) this.lexical(p);
    const formatted = new Set<string>();
    const skipped = new Set<string>();

    const nodeFiles = paths.filter((p) => /\.(js|jsx|ts|tsx|json|css|scss|md|yaml|yml)$/.test(p));
    if (nodeFiles.length) {
      const prettier = path.join(this.root, "node_modules", ".bin", process.platform === "win32" ? "prettier.cmd" : "prettier");
      const biome = path.join(this.root, "node_modules", ".bin", process.platform === "win32" ? "biome.cmd" : "biome");
      if (await executable(prettier)) {
        const result = await run(prettier, ["--write", ...nodeFiles], this.root);
        if (result.code === 0) nodeFiles.forEach((p) => formatted.add(p)); else nodeFiles.forEach((p) => skipped.add(p));
      } else if (await executable(biome)) {
        const result = await run(biome, ["format", "--write", ...nodeFiles], this.root);
        if (result.code === 0) nodeFiles.forEach((p) => formatted.add(p)); else nodeFiles.forEach((p) => skipped.add(p));
      } else nodeFiles.forEach((p) => skipped.add(p));
    }

    const groups: Array<{ ext: RegExp; command: string; args: (p: string) => string[] }> = [
      { ext: /\.py$/, command: "ruff", args: (p) => ["format", p] },
      { ext: /\.rs$/, command: "rustfmt", args: (p) => [p] },
      { ext: /\.go$/, command: "gofmt", args: (p) => ["-w", p] },
      { ext: /\.(c|cc|cpp|cxx|h|hpp)$/, command: "clang-format", args: (p) => ["-i", p] },
    ];
    for (const group of groups) {
      const candidates = paths.filter((p) => group.ext.test(p) && !formatted.has(p));
      if (!candidates.length) continue;
      const available = await this.findOnPath(group.command);
      if (!available) { candidates.forEach((p) => skipped.add(p)); continue; }
      for (const p of candidates) {
        const result = await run(group.command, group.args(p), this.root);
        if (result.code === 0) formatted.add(p); else skipped.add(p);
      }
    }
    return { formatted: [...formatted], skipped: [...skipped] };
  }

  private async findOnPath(command: string) {
    for (const dir of (process.env.PATH ?? "").split(path.delimiter)) {
      for (const suffix of process.platform === "win32" ? [".exe", ".cmd", ".bat", ""] : [""]) {
        if (await executable(path.join(dir, `${command}${suffix}`))) return true;
      }
    }
    return false;
  }
}
