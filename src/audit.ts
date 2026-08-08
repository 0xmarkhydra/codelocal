import path from "node:path";
import { appendPrivateJsonl, DEFAULT_STATE_DIR } from "./state.js";
import { redactCommand } from "./security-policy.js";

export type AuditEvent = {
  ts?: string;
  event: string;
  requestId?: string;
  workspaceKey?: string;
  tool?: string;
  processId?: string;
  approvalId?: string;
  status?: string;
  riskLevel?: string;
  detail?: unknown;
};

const AUDIT_ENABLED = process.env.CODELOCAL_AUDIT_FILE !== "0";
const AUDIT_FILE = process.env.CODELOCAL_AUDIT_PATH ?? path.join(DEFAULT_STATE_DIR, "audit.jsonl");

function sanitize(value: unknown, key = ""): unknown {
  if (value instanceof Error) return { name: value.name, message: value.message };
  if (Array.isArray(value)) return value.map((item) => sanitize(item));
  if (typeof value === "string") {
    if (/command|detail|query/i.test(key)) return redactCommand(value).slice(0, 2000);
    if (/content|patch|input|oldText|newText/i.test(key)) return `[${Buffer.byteLength(value, "utf8")} bytes]`;
    return value.slice(0, 4000);
  }
  if (!value || typeof value !== "object") return value;
  const out: Record<string, unknown> = {};
  for (const [childKey, childValue] of Object.entries(value as Record<string, unknown>)) {
    if (/token|secret|password|authorization|cookie|credentialSecret/i.test(childKey)) out[childKey] = "[REDACTED]";
    else out[childKey] = sanitize(childValue, childKey);
  }
  return out;
}

export async function audit(event: AuditEvent) {
  if (!AUDIT_ENABLED) return;
  const record = sanitize({ ts: event.ts ?? new Date().toISOString(), ...event }) as Record<string, unknown>;
  await appendPrivateJsonl(AUDIT_FILE, record).catch(() => undefined);
}
