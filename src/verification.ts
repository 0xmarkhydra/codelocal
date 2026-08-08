import { randomUUID } from "node:crypto";
import { spawn } from "node:child_process";
import type { SemanticRouter } from "./semantic-router.js";
import type { ProjectContextEngine } from "./context-engine.js";

function diagnosticKey(d: any) {
  return `${d.path ?? ""}:${d.line ?? 0}:${d.column ?? 0}:${d.code ?? ""}:${d.message ?? ""}`;
}

async function gitDiff(root: string) {
  return new Promise<string>((resolve) => {
    const child = spawn("git", ["diff", "--no-ext-diff", "--unified=2"], { cwd: root, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", (d) => { output = (output + d.toString()).slice(-1_500_000); });
    child.stderr.on("data", (d) => { output = (output + d.toString()).slice(-1_500_000); });
    child.on("error", () => resolve(""));
    child.on("close", () => resolve(output));
  });
}

export class VerificationEngine {
  private snapshots = new Map<string, any[]>();

  constructor(private root: string, private semantic: SemanticRouter, private context: ProjectContextEngine) {}

  async snapshotDiagnostics(paths: string[] = []) {
    const diagnostics: any[] = [];
    if (paths.length) {
      for (const p of [...new Set(paths)]) diagnostics.push(...await this.semantic.diagnostics(p, 500));
    } else diagnostics.push(...await this.semantic.diagnostics(undefined, 1000));
    const snapshotId = randomUUID();
    this.snapshots.set(snapshotId, diagnostics);
    if (this.snapshots.size > 30) this.snapshots.delete(this.snapshots.keys().next().value as string);
    return { snapshotId, diagnostics };
  }

  compare(baselineId: string, currentDiagnostics: any[]) {
    const baseline = this.snapshots.get(baselineId);
    if (!baseline) throw new Error("Unknown diagnostic baseline.");
    const before = new Map(baseline.map((d) => [diagnosticKey(d), d]));
    const after = new Map(currentDiagnostics.map((d) => [diagnosticKey(d), d]));
    return {
      new: [...after.entries()].filter(([k]) => !before.has(k)).map(([, d]) => d),
      persisting: [...after.entries()].filter(([k]) => before.has(k)).map(([, d]) => d),
      resolved: [...before.entries()].filter(([k]) => !after.has(k)).map(([, d]) => d),
    };
  }

  async verify(paths: string[] = [], baselineId?: string) {
    const current = await this.snapshotDiagnostics(paths);
    const project = await this.context.map();
    const diff = await gitDiff(this.root);
    const checks = [...project.typecheckCommands, ...project.lintCommands, ...project.testCommands].slice(0, 8);
    return {
      diagnostics: current.diagnostics,
      diagnosticSnapshotId: current.snapshotId,
      regression: baselineId ? this.compare(baselineId, current.diagnostics) : null,
      recommendedChecks: checks,
      diff: diff.slice(-1_000_000),
      diffTruncated: diff.length > 1_000_000,
    };
  }
}
