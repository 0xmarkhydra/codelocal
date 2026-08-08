export type RiskLevel = "SAFE" | "REVIEW" | "HIGH" | "CRITICAL" | "BLOCKED";
export type NetworkPolicy = "deny" | "approval" | "allow";

export type PolicyDecision = {
  riskLevel: RiskLevel;
  matchedRules: string[];
  requiresApproval: boolean;
  blocked: boolean;
  redactedCommand: string;
  reason: string;
};

const rank: Record<RiskLevel, number> = { SAFE: 0, REVIEW: 1, HIGH: 2, CRITICAL: 3, BLOCKED: 4 };

export function redactCommand(command: string) {
  let value = command;
  value = value.replace(/(authorization\s*:\s*bearer\s+)[^\s"']+/gi, "$1[REDACTED]");
  value = value.replace(/((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret|cookie)\s*[=:]\s*)[^\s"']+/gi, "$1[REDACTED]");
  value = value.replace(/(--(?:token|password|secret|api-key|apikey)\s+)([^\s]+)/gi, "$1[REDACTED]");
  value = value.replace(/((?:OPENAI_API_KEY|CODEX_API_KEY|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN|GITHUB_TOKEN|GH_TOKEN|NPM_TOKEN)=)([^\s]+)/gi, "$1[REDACTED]");
  value = value.replace(/(https?:\/\/[^\s:@]+:)[^@\s]+@/gi, "$1[REDACTED]@");
  return value;
}

export function isSensitivePath(relativePath: string) {
  const p = relativePath.replace(/\\/g, "/").replace(/^\.\//, "");
  const base = p.split("/").pop()?.toLowerCase() ?? "";
  if ([".env.example", ".env.sample", ".env.template"].includes(base)) return false;
  if (/(^|\/)\.(ssh|aws|gnupg|gcloud|azure)(\/|$)/i.test(p)) return true;
  if (/(^|\/)\.env($|\.)/i.test(p)) return true;
  if (/\.(pem|p12|pfx|key|kdbx)$/i.test(base)) return true;
  if (/(credentials?|service[-_]?account|private[-_]?key|secrets?)\.(json|ya?ml|toml|ini)$/i.test(base)) return true;
  return false;
}

export function classifyCommand(command: string, networkPolicy: NetworkPolicy = "approval"): PolicyDecision {
  const normalized = command.replace(/\s+/g, " ").trim();
  const rules: string[] = [];
  let risk: RiskLevel = "SAFE";
  let blocked = false;
  let approval = false;

  const hit = (pattern: RegExp, reason: string, level: RiskLevel, options: { block?: boolean; approve?: boolean } = {}) => {
    if (!pattern.test(normalized)) return;
    rules.push(reason);
    if (rank[level] > rank[risk]) risk = level;
    blocked ||= !!options.block;
    approval ||= !!options.approve;
  };

  hit(/(^|[;&|]\s*)sudo\b/i, "privilege escalation", "BLOCKED", { block: true });
  hit(/(^|[;&|]\s*)(shutdown|reboot|halt|diskutil|mkfs|fdisk|gpt|mount|umount)\b/i, "system or disk administration", "BLOCKED", { block: true });
  hit(/\bdd\s+[^\n]*\bof=\/dev\//i, "raw device write", "BLOCKED", { block: true });
  hit(/(^|\s)(~\/)?\.(ssh|aws|gnupg|gcloud|azure)(\/|\s|$)/i, "credential directory access", "BLOCKED", { block: true });
  hit(/(^|\s)\/dev\/(?!null\b|zero\b|random\b|urandom\b)/i, "device file access", "BLOCKED", { block: true });
  hit(/(^|\s)\.\.\/(?:\.\.\/)?/, "parent-directory traversal", "CRITICAL", { block: true });
  hit(/\bgit\s+(reset\s+--hard|clean\s+-[a-z]*f|push\s+.*--force)\b/i, "destructive Git action", "HIGH", { approve: true });
  hit(/\brm\s+-[^\s]*r/i, "recursive delete", "HIGH", { approve: true });
  hit(/\b(prisma|typeorm|sequelize|knex|alembic|rails)\b[^\n]*(migrate|migration|db:)/i, "database migration", "HIGH", { approve: true });
  hit(/\b(git\s+(commit|push|tag|merge|rebase))\b/i, "Git write action", "REVIEW", { approve: true });
  hit(/\b(npm|pnpm|yarn|bun|pip|pipx|poetry|uv|cargo|go)\s+(install|add|remove|uninstall|update|upgrade|get)\b/i, "dependency or toolchain change", "REVIEW", { approve: true });
  const networkPattern = /\b(curl|wget|ssh|scp|sftp|ftp|nc|ncat|telnet)\b/i;
  if (networkPattern.test(normalized)) {
    if (networkPolicy === "deny") hit(networkPattern, "network access denied by policy", "BLOCKED", { block: true });
    else if (networkPolicy === "approval") hit(networkPattern, "network or remote command", "REVIEW", { approve: true });
  }
  hit(/\b(chmod\s+[0-7]*[67][0-7][0-7]|chown|launchctl|systemctl|service)\b/i, "permission or service modification", "HIGH", { approve: true });

  if (blocked) risk = "BLOCKED";
  const reason = rules.length ? rules.join("; ") : "no risky policy rule matched";
  return {
    riskLevel: risk,
    matchedRules: [...new Set(rules)],
    requiresApproval: !blocked && approval,
    blocked,
    redactedCommand: redactCommand(normalized),
    reason,
  };
}

export function classifyGitWrite(operation: string, detail = ""): PolicyDecision {
  return classifyCommand(`git ${operation} ${detail}`.trim(), "approval");
}

export function decisionSummary(decision: PolicyDecision) {
  return {
    riskLevel: decision.riskLevel,
    matchedRules: decision.matchedRules,
    requiresApproval: decision.requiresApproval,
    blocked: decision.blocked,
    redactedCommand: decision.redactedCommand,
    reason: decision.reason,
  };
}
