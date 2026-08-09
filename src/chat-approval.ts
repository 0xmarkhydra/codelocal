import { createHash, randomUUID, timingSafeEqual } from "node:crypto";
import type { PolicyDecision } from "./security-policy.js";

export type CommandPreflight = {
  status: "safe" | "approval_required" | "blocked";
  riskLevel: PolicyDecision["riskLevel"];
  reason: string;
  matchedRules: string[];
  command: string;
  approvalToken?: string;
  expiresAt?: number;
};

type PendingApproval = {
  tokenHash: string;
  fingerprint: string;
  expiresAt: number;
};

function hash(value: string) {
  return createHash("sha256").update(value).digest("hex");
}

function sameHex(a: string, b: string) {
  const aa = Buffer.from(a, "hex");
  const bb = Buffer.from(b, "hex");
  return aa.length === bb.length && timingSafeEqual(aa, bb);
}

export class ChatApprovalBroker {
  private pending = new Map<string, PendingApproval>();

  constructor(private ttlMs = Number(process.env.CODELOCAL_CHAT_APPROVAL_TTL_MS ?? 5 * 60_000)) {}

  private fingerprint(command: string, cwd: string, decision: PolicyDecision) {
    return hash(JSON.stringify({
      rawCommandHash: hash(command),
      redactedCommand: decision.redactedCommand,
      cwd,
      rules: [...decision.matchedRules].sort(),
      risk: decision.riskLevel,
    }));
  }

  private prune() {
    const now = Date.now();
    for (const [id, approval] of this.pending) if (approval.expiresAt <= now) this.pending.delete(id);
  }

  preflight(command: string, cwd: string, decision: PolicyDecision): CommandPreflight {
    this.prune();
    if (decision.blocked) return { status: "blocked", riskLevel: decision.riskLevel, reason: decision.reason, matchedRules: decision.matchedRules, command: decision.redactedCommand };
    if (!decision.requiresApproval) return { status: "safe", riskLevel: decision.riskLevel, reason: decision.reason, matchedRules: decision.matchedRules, command: decision.redactedCommand };

    const approvalToken = randomUUID() + randomUUID().replaceAll("-", "");
    const id = randomUUID();
    const expiresAt = Date.now() + this.ttlMs;
    this.pending.set(id, { tokenHash: hash(approvalToken), fingerprint: this.fingerprint(command, cwd, decision), expiresAt });
    return {
      status: "approval_required",
      riskLevel: decision.riskLevel,
      reason: decision.reason,
      matchedRules: decision.matchedRules,
      command: decision.redactedCommand,
      approvalToken: `${id}.${approvalToken}`,
      expiresAt,
    };
  }

  consume(approvalToken: string | undefined, command: string, cwd: string, decision: PolicyDecision) {
    this.prune();
    if (!decision.requiresApproval || decision.blocked) return !decision.blocked;
    if (!approvalToken) return false;
    const dot = approvalToken.indexOf(".");
    if (dot <= 0) return false;
    const id = approvalToken.slice(0, dot);
    const secret = approvalToken.slice(dot + 1);
    const pending = this.pending.get(id);
    if (!pending) return false;
    this.pending.delete(id);
    if (pending.expiresAt <= Date.now()) return false;
    if (!sameHex(pending.tokenHash, hash(secret))) return false;
    return sameHex(pending.fingerprint, this.fingerprint(command, cwd, decision));
  }
}
