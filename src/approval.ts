import { createInterface } from "node:readline/promises";
import { randomUUID } from "node:crypto";
import type { PolicyDecision } from "./security-policy.js";
import { audit } from "./audit.js";

export type ApprovalMode = "prompt" | "deny" | "auto-safe";

export type ApprovalRequest = {
  approvalId: string;
  requestId?: string;
  operation: string;
  riskLevel: string;
  workspace: string;
  summary: string;
  redactedDetail: string;
  matchedRules: string[];
  expiresAt: number;
};

export class ApprovalEngine {
  private sessionAllowedRules = new Set<string>();

  constructor(
    private workspace: string,
    private mode: ApprovalMode = "prompt",
    private ttlMs = 2 * 60_000,
  ) {}

  allowRuleForSession(rule: string) {
    this.sessionAllowedRules.add(rule);
  }

  clearSessionRules() {
    this.sessionAllowedRules.clear();
  }

  private canUseSessionRule(decision: PolicyDecision) {
    return decision.matchedRules.length > 0 && decision.matchedRules.every((rule) => this.sessionAllowedRules.has(rule));
  }

  async approve(operation: string, detail: string, decision: PolicyDecision, requestId?: string) {
    if (decision.blocked) return false;
    if (!decision.requiresApproval) return true;
    if (this.canUseSessionRule(decision)) return true;
    if (this.mode === "deny") return false;
    if (this.mode === "auto-safe" && decision.riskLevel !== "SAFE") return false;
    if (!process.stdin.isTTY || !process.stdout.isTTY) return false;

    const request: ApprovalRequest = {
      approvalId: randomUUID(),
      requestId,
      operation,
      riskLevel: decision.riskLevel,
      workspace: this.workspace,
      summary: decision.reason,
      redactedDetail: detail,
      matchedRules: decision.matchedRules,
      expiresAt: Date.now() + this.ttlMs,
    };

    await audit({
      event: "approval.requested",
      requestId,
      workspaceKey: this.workspace,
      approvalId: request.approvalId,
      riskLevel: request.riskLevel,
      detail: request,
    });

    const rl = createInterface({ input: process.stdin, output: process.stdout });
    try {
      const rules = request.matchedRules.length ? request.matchedRules.map((r) => `  - ${r}`).join("\n") : "  - policy review";
      const answer = await rl.question(
        `\n[CodeLocal approval required]\n` +
        `Operation: ${request.operation}\n` +
        `Risk: ${request.riskLevel}\n` +
        `Workspace: ${request.workspace}\n` +
        `Detail: ${request.redactedDetail}\n` +
        `Rules:\n${rules}\n` +
        `Allow once [y], allow matched rules for this session [s], or deny [N]? `,
      );
      const normalized = answer.trim().toLowerCase();
      const approved = normalized === "y" || normalized === "yes" || normalized === "s" || normalized === "session";
      if (normalized === "s" || normalized === "session") {
        for (const rule of request.matchedRules) this.sessionAllowedRules.add(rule);
      }
      await audit({
        event: approved ? "approval.approved" : "approval.denied",
        requestId,
        workspaceKey: this.workspace,
        approvalId: request.approvalId,
        riskLevel: request.riskLevel,
        status: approved ? "approved" : "denied",
      });
      return approved;
    } finally {
      rl.close();
    }
  }
}
