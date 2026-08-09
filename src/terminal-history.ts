import path from "node:path";
import { promises as fs } from "node:fs";
import { appendPrivateJsonl, DEFAULT_STATE_DIR } from "./state.js";
import { redactCommand } from "./security-policy.js";
import type { ProcessRecord } from "./process-manager.js";

export type TerminalHistoryRecord = {
  ts: string;
  event: "started" | "finished";
  workspaceKey: string;
  processId: string;
  requestId?: string;
  sessionId?: string;
  cwd: string;
  command: string;
  riskLevel: string;
  matchedRules: string[];
  approval: "automatic" | "chat";
  startedAt: number;
  finishedAt?: number;
  durationMs?: number;
  exitCode?: number | null;
  status?: string;
  executionMode?: string;
};

type StartInput = {
  workspaceKey: string;
  processId: string;
  requestId?: string;
  sessionId?: string;
  cwd: string;
  command: string;
  riskLevel: string;
  matchedRules: string[];
  approval: "automatic" | "chat";
  startedAt: number;
  executionMode?: string;
};

export class TerminalHistory {
  private starts = new Map<string, TerminalHistoryRecord>();

  constructor(private file = process.env.CODELOCAL_TERMINAL_HISTORY_PATH ?? path.join(DEFAULT_STATE_DIR, "terminal-history.jsonl")) {}

  async started(input: StartInput) {
    const record: TerminalHistoryRecord = {
      ts: new Date().toISOString(),
      event: "started",
      workspaceKey: input.workspaceKey,
      processId: input.processId,
      requestId: input.requestId,
      sessionId: input.sessionId,
      cwd: input.cwd,
      command: redactCommand(input.command).slice(0, 4000),
      riskLevel: input.riskLevel,
      matchedRules: input.matchedRules,
      approval: input.approval,
      startedAt: input.startedAt,
      executionMode: input.executionMode,
    };
    this.starts.set(record.processId, record);
    await appendPrivateJsonl(this.file, record);
    return record;
  }

  async finished(record: ProcessRecord) {
    const start = this.starts.get(record.processId);
    const finishedAt = Date.now();
    const output: TerminalHistoryRecord = {
      ts: new Date().toISOString(),
      event: "finished",
      workspaceKey: record.workspaceKey,
      processId: record.processId,
      requestId: start?.requestId,
      sessionId: start?.sessionId,
      cwd: start?.cwd ?? record.cwd,
      command: start?.command ?? redactCommand(record.command).slice(0, 4000),
      riskLevel: start?.riskLevel ?? "UNKNOWN",
      matchedRules: start?.matchedRules ?? [],
      approval: start?.approval ?? "automatic",
      startedAt: record.startedAt,
      finishedAt,
      durationMs: Math.max(0, finishedAt - record.startedAt),
      exitCode: record.exitCode,
      status: record.status,
      executionMode: record.executionMode,
    };
    this.starts.delete(record.processId);
    await appendPrivateJsonl(this.file, output);
    return output;
  }

  private async tail(maxBytes = 4 * 1024 * 1024) {
    let handle: fs.FileHandle | null = null;
    try {
      handle = await fs.open(this.file, "r");
      const stat = await handle.stat();
      const length = Math.min(stat.size, maxBytes);
      const buffer = Buffer.alloc(length);
      await handle.read(buffer, 0, length, Math.max(0, stat.size - length));
      let text = buffer.toString("utf8");
      if (stat.size > length) text = text.slice(text.indexOf("\n") + 1);
      return text;
    } catch {
      return "";
    } finally {
      await handle?.close().catch(() => undefined);
    }
  }

  async query(options: { workspaceKey?: string; query?: string; limit?: number; event?: "started" | "finished" | "all" } = {}) {
    const raw = await this.tail();
    const query = String(options.query ?? "").trim().toLowerCase();
    const event = options.event ?? "started";
    const limit = Math.max(1, Math.min(Number(options.limit ?? 50), 500));
    const records: TerminalHistoryRecord[] = [];
    for (const line of raw.split(/\r?\n/).reverse()) {
      if (!line.trim()) continue;
      try {
        const record = JSON.parse(line) as TerminalHistoryRecord;
        if (options.workspaceKey && record.workspaceKey !== options.workspaceKey) continue;
        if (event !== "all" && record.event !== event) continue;
        if (query && !`${record.command} ${record.cwd} ${record.riskLevel} ${record.status ?? ""}`.toLowerCase().includes(query)) continue;
        records.push(record);
        if (records.length >= limit) break;
      } catch {}
    }
    return { records, truncatedByReadWindow: Buffer.byteLength(raw, "utf8") >= 4 * 1024 * 1024 };
  }
}
