type RiskLevel = "SAFE" | "REVIEW" | "HIGH" | "CRITICAL";

type AuditRecord = {
  command: string;
  redactedCommand: string;
  riskLevel: RiskLevel;
  rules: string[];
};

const requests = new Map<string, AuditRecord>();

function redactCommand(command: string) {
  let value = command;
  value = value.replace(/(authorization\s*:\s*bearer\s+)[^\s"']+/gi, "$1[REDACTED]");
  value = value.replace(/((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[=:]\s*)[^\s"']+/gi, "$1[REDACTED]");
  value = value.replace(/(--(?:token|password|secret|api-key|apikey)\s+)([^\s]+)/gi, "$1[REDACTED]");
  value = value.replace(/((?:OPENAI_API_KEY|CODEX_API_KEY|AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|GH_TOKEN)=)([^\s]+)/gi, "$1[REDACTED]");
  return value;
}

function classify(command: string): AuditRecord {
  const normalized = command.replace(/\s+/g, " ").trim();
  const rules: string[] = [];
  let risk: RiskLevel = "SAFE";

  const hit = (pattern: RegExp, reason: string, level: RiskLevel) => {
    if (!pattern.test(normalized)) return;
    rules.push(reason);
    const rank: Record<RiskLevel, number> = { SAFE: 0, REVIEW: 1, HIGH: 2, CRITICAL: 3 };
    if (rank[level] > rank[risk]) risk = level;
  };

  hit(/(^|[;&|]\s*)sudo\b/i, "sudo/system privilege escalation", "CRITICAL");
  hit(/(^|[;&|]\s*)(shutdown|reboot|halt|diskutil|mkfs|fdisk|gpt|mount|umount)\b/i, "system/disk administration", "CRITICAL");
  hit(/\bdd\s+[^\n]*\bof=\/dev\//i, "raw device write", "CRITICAL");
  hit(/(^|\s)(~\/)?\.(ssh|aws|gnupg|gcloud)(\/|\s|$)/i, "credential directory access", "CRITICAL");
  hit(/(^|\s)\.\.\/(?:\.\.\/)?/, "parent-directory traversal", "HIGH");
  hit(/\bgit\s+(reset\s+--hard|clean\s+-[a-z]*f)\b/i, "destructive Git action", "HIGH");
  hit(/\brm\s+-[^\s]*r/i, "recursive delete", "HIGH");
  hit(/\b(prisma|typeorm|sequelize|knex|alembic|rails)\b[^\n]*(migrate|migration|db:)/i, "database migration", "HIGH");
  hit(/\bgit\s+(commit|push)\b/i, "Git write action", "REVIEW");
  hit(/\b(npm|pnpm|yarn|bun)\s+(install|add|remove|uninstall|update|upgrade)\b/i, "package dependency change", "REVIEW");
  hit(/\b(curl|wget|ssh|scp|sftp)\b/i, "network/remote command", "REVIEW");

  return { command: normalized, redactedCommand: redactCommand(normalized), riskLevel: risk, rules };
}

function panel(title: string, record: AuditRecord, action: string) {
  const rules = record.rules.length ? record.rules.map((r) => `  - ${r}`).join("\n") : "  - policy decision";
  return `\n${title}\nRisk: ${record.riskLevel}\nCommand: ${record.redactedCommand}\nMatched rules:\n${rules}\nAction: ${action}\n`;
}

const original = {
  log: console.log.bind(console),
  warn: console.warn.bind(console),
  error: console.error.bind(console),
};

function inspect(args: unknown[]) {
  if (args.length !== 1 || typeof args[0] !== "string") return;
  const line = args[0];
  if (!line.startsWith("{")) return;
  try {
    const event = JSON.parse(line) as Record<string, any>;
    if (event.event === "tool.received" && event.tool === "run_command" && event.requestId && event.args?.command) {
      const record = classify(String(event.args.command));
      requests.set(String(event.requestId), record);
      if (record.riskLevel !== "SAFE") {
        const action = record.riskLevel === "CRITICAL" ? "policy will block unless dangerous override is explicitly enabled" : "local approval/policy review required";
        original.warn(panel("⚠️  [CodeLocal SECURITY] Suspicious command detected", record, action));
      }
    }
    if (event.event === "tool.failed" && event.requestId) {
      const record = requests.get(String(event.requestId));
      const message = typeof event.error === "string" ? event.error : event.error?.message;
      if (record && /command blocked|denied by local approval|sensitive-path|policy/i.test(String(message ?? ""))) {
        original.error(panel("⛔ [CodeLocal SECURITY] Command was NOT executed", record, String(message ?? "Blocked by policy")));
      }
      requests.delete(String(event.requestId));
    }
    if (event.event === "tool.completed" && event.requestId) {
      const record = requests.get(String(event.requestId));
      if (record && record.riskLevel !== "SAFE") {
        original.warn(panel("✅ [CodeLocal SECURITY] Reviewed command completed", record, "executed after current local policy allowed it"));
      }
      requests.delete(String(event.requestId));
    }
  } catch {
    // Not a structured CodeLocal log line.
  }
}

console.log = (...args: unknown[]) => { inspect(args); original.log(...args); };
console.warn = (...args: unknown[]) => { inspect(args); original.warn(...args); };
console.error = (...args: unknown[]) => { inspect(args); original.error(...args); };

await import("./client.js");
