import { createHash, randomUUID } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";
import { DEFAULT_STATE_DIR, readJsonFile, writeJsonAtomic } from "./state.js";
import type { PolicyDecision } from "./security-policy.js";

export type RememberedApproval = {
  id: string;
  workspaceKey: string;
  actionKey: string;
  label: string;
  redactedCommand: string;
  riskLevel: string;
  matchedRules: string[];
  createdAt: number;
  lastUsedAt: number;
  useCount: number;
};

type ApprovalFile = {
  version: 1;
  workspaceKey: string;
  approvals: RememberedApproval[];
};

function workspaceHash(workspaceKey: string) {
  return createHash("sha256").update(workspaceKey).digest("hex").slice(0, 24);
}

export class ApprovalMemory {
  constructor(private rootDir = path.join(DEFAULT_STATE_DIR, "approvals")) {}

  private fileFor(workspaceKey: string) {
    return path.join(this.rootDir, `${workspaceHash(workspaceKey)}.json`);
  }

  private async read(workspaceKey: string): Promise<ApprovalFile> {
    const value = await readJsonFile<ApprovalFile>(this.fileFor(workspaceKey), { version: 1, workspaceKey, approvals: [] });
    const approvals = Array.isArray(value.approvals)
      ? value.approvals.filter((entry) => entry?.workspaceKey === workspaceKey && typeof entry.actionKey === "string")
      : [];
    return { version: 1, workspaceKey, approvals };
  }

  private async write(workspaceKey: string, approvals: RememberedApproval[]) {
    const unique = new Map<string, RememberedApproval>();
    for (const entry of approvals) unique.set(entry.actionKey, entry);
    await writeJsonAtomic(this.fileFor(workspaceKey), {
      version: 1,
      workspaceKey,
      approvals: [...unique.values()].sort((a, b) => b.lastUsedAt - a.lastUsedAt),
    });
  }

  async find(workspaceKey: string, actionKey: string) {
    const data = await this.read(workspaceKey);
    return data.approvals.find((entry) => entry.actionKey === actionKey) ?? null;
  }

  async remember(workspaceKey: string, decision: PolicyDecision) {
    if (decision.approvalPolicy !== "rememberable" || !decision.approvalKey) return null;
    const data = await this.read(workspaceKey);
    const previous = data.approvals.find((entry) => entry.actionKey === decision.approvalKey);
    const now = Date.now();
    const entry: RememberedApproval = {
      id: previous?.id ?? randomUUID(),
      workspaceKey,
      actionKey: decision.approvalKey,
      label: decision.approvalLabel ?? decision.redactedCommand,
      redactedCommand: decision.redactedCommand,
      riskLevel: decision.riskLevel,
      matchedRules: [...decision.matchedRules],
      createdAt: previous?.createdAt ?? now,
      lastUsedAt: now,
      useCount: (previous?.useCount ?? 0) + 1,
    };
    await this.write(workspaceKey, [...data.approvals.filter((item) => item.actionKey !== entry.actionKey), entry]);
    return entry;
  }

  async touch(workspaceKey: string, actionKey: string) {
    const data = await this.read(workspaceKey);
    const entry = data.approvals.find((item) => item.actionKey === actionKey);
    if (!entry) return null;
    entry.lastUsedAt = Date.now();
    entry.useCount = Math.max(0, Number(entry.useCount) || 0) + 1;
    await this.write(workspaceKey, data.approvals);
    return entry;
  }

  async list(workspaceKey?: string) {
    if (workspaceKey) return (await this.read(workspaceKey)).approvals;
    const names = await fs.readdir(this.rootDir).catch(() => [] as string[]);
    const output: RememberedApproval[] = [];
    for (const name of names) {
      if (!name.endsWith(".json")) continue;
      const value = await readJsonFile<ApprovalFile>(path.join(this.rootDir, name), { version: 1, workspaceKey: "", approvals: [] });
      if (!value.workspaceKey || !Array.isArray(value.approvals)) continue;
      output.push(...value.approvals.filter((entry) => entry?.workspaceKey === value.workspaceKey && typeof entry.actionKey === "string"));
    }
    return output.sort((a, b) => b.lastUsedAt - a.lastUsedAt);
  }

  async revoke(identifier: string, workspaceKey?: string) {
    const targets = workspaceKey ? [workspaceKey] : [...new Set((await this.list()).map((entry) => entry.workspaceKey))];
    let removed = 0;
    for (const key of targets) {
      const data = await this.read(key);
      const next = data.approvals.filter((entry) => entry.id !== identifier && entry.actionKey !== identifier);
      removed += data.approvals.length - next.length;
      if (next.length !== data.approvals.length) await this.write(key, next);
    }
    return removed;
  }

  async reset(workspaceKey?: string) {
    if (workspaceKey) {
      const data = await this.read(workspaceKey);
      await fs.rm(this.fileFor(workspaceKey), { force: true });
      return data.approvals.length;
    }
    const count = (await this.list()).length;
    await fs.rm(this.rootDir, { recursive: true, force: true });
    return count;
  }
}
