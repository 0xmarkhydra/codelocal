import { randomUUID } from "node:crypto";

export const PROTOCOL_VERSION = 2;
export const MIN_PROTOCOL_VERSION = 1;

export type ErrorCode =
  | "BAD_REQUEST"
  | "CLIENT_UPGRADE_REQUIRED"
  | "CLIENT_OFFLINE"
  | "WORKSPACE_REQUIRED"
  | "WORKSPACE_OFFLINE"
  | "TOOL_TIMEOUT"
  | "TOOL_CANCELLED"
  | "TOOL_FAILED"
  | "POLICY_BLOCKED"
  | "APPROVAL_DENIED"
  | "SENSITIVE_PATH"
  | "PATH_ESCAPE"
  | "CONFLICT"
  | "NOT_FOUND"
  | "UNSUPPORTED"
  | "AUTH_FAILED"
  | "DUPLICATE_REQUEST";

export type ClientCapabilities = {
  filesystem: boolean;
  git: boolean;
  shell: boolean;
  pty: boolean;
  sandbox: "native" | "best-effort" | "policy-only" | "none";
  semanticProviders: string[];
  idempotency: boolean;
  cancellation: boolean;
  approvals: boolean;
  approvalMemory?: boolean;
  hostPolicyExecution?: boolean;
  mcpHub?: boolean;
};

export type RegisterMessage = {
  type: "register";
  protocolVersion: number;
  token?: string;
  credentialId?: string;
  credentialSecret?: string;
  deviceId: string;
  deviceName?: string;
  workspaceId: string;
  workspaceName: string;
  projectRoot?: string;
  capabilities: ClientCapabilities;
};

export type ToolCallMessage = {
  type: "tool_call";
  protocolVersion: number;
  requestId: string;
  sessionId?: string;
  workspaceKey: string;
  tool: string;
  args: unknown;
  idempotencyKey?: string;
  deadline?: number;
};

export type ToolCancelMessage = {
  type: "tool_cancel";
  protocolVersion: number;
  requestId: string;
  reason?: string;
};

export type ToolResultMessage = {
  type: "tool_result";
  protocolVersion: number;
  requestId: string;
  ok: boolean;
  result?: unknown;
  errorCode?: ErrorCode;
  errorMessage?: string;
  metadata?: Record<string, unknown>;
};

export type ServerMessage =
  | ToolCallMessage
  | ToolCancelMessage
  | { type: "registered"; protocolVersion: number; serverCapabilities: Record<string, unknown> }
  | { type: "ping"; ts: number };

export function requestId() {
  return randomUUID();
}

export function normalizeError(error: unknown): { errorCode: ErrorCode; errorMessage: string } {
  const message = error instanceof Error ? error.message : String(error);
  const lower = message.toLowerCase();
  if (lower.includes("sensitive-path")) return { errorCode: "SENSITIVE_PATH", errorMessage: message };
  if (lower.includes("escapes project_root") || lower.includes("path escape") || lower.includes("unsafe patch path")) return { errorCode: "PATH_ESCAPE", errorMessage: message };
  if (lower.includes("changed since read") || lower.includes("hash mismatch") || lower.includes("conflict")) return { errorCode: "CONFLICT", errorMessage: message };
  if (lower.includes("approval") && (lower.includes("denied") || lower.includes("rejected"))) return { errorCode: "APPROVAL_DENIED", errorMessage: message };
  if (lower.includes("blocked") || lower.includes("policy")) return { errorCode: "POLICY_BLOCKED", errorMessage: message };
  if (lower.includes("not found") || lower.includes("enoent") || lower.includes("unknown process")) return { errorCode: "NOT_FOUND", errorMessage: message };
  if (lower.includes("unsupported") || lower.includes("not available")) return { errorCode: "UNSUPPORTED", errorMessage: message };
  if (lower.includes("cancel")) return { errorCode: "TOOL_CANCELLED", errorMessage: message };
  return { errorCode: "TOOL_FAILED", errorMessage: message };
}

export function protocolCompatible(version: unknown) {
  return typeof version === "number" && version >= MIN_PROTOCOL_VERSION && version <= PROTOCOL_VERSION;
}

export function isSideEffectingTool(tool: string) {
  return new Set([
    "write_file",
    "edit_file",
    "apply_patch",
    "apply_edits",
    "format_changed_files",
    "run_command",
    "exec_start",
    "pty_start",
    "process_write",
    "process_kill",
    "exec_cancel",
    "git_stage",
    "git_unstage",
    "git_commit",
    "git_push",
    "approval_revoke",
    "approval_reset",
    "mcp_call",
  ]).has(tool);
}
