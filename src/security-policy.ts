import { createHash } from "node:crypto";
import os from "node:os";
import path from "node:path";

export type RiskLevel = "SAFE" | "REVIEW" | "HIGH" | "CRITICAL" | "BLOCKED";
export type NetworkPolicy = "deny" | "approval" | "allow";
export type ApprovalPolicy = "none" | "rememberable" | "always" | "blocked";

export type CommandSecurityContext = {
  workspaceRoot?: string;
  cwd?: string;
};

export type PolicyDecision = {
  riskLevel: RiskLevel;
  matchedRules: string[];
  requiresApproval: boolean;
  blocked: boolean;
  redactedCommand: string;
  reason: string;
  approvalPolicy: ApprovalPolicy;
  approvalKey?: string;
  approvalLabel?: string;
};

const rank: Record<RiskLevel, number> = { SAFE: 0, REVIEW: 1, HIGH: 2, CRITICAL: 3, BLOCKED: 4 };
const SECRET_ENV_NAME = /(?:^|_)(?:API_?KEY|TOKEN|SECRET|PASSWORD|PASSWD|PRIVATE_?KEY|ACCESS_?KEY|SESSION_?TOKEN|COOKIE)(?:$|_)/i;

function hashKey(value: string) {
  return createHash("sha256").update(value).digest("hex").slice(0, 24);
}

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
  if (/(^|\/)\.(git|ssh|aws|gnupg|gcloud|azure)(\/|$)/i.test(p)) return true;
  if (/(^|\/)\.env($|\.)/i.test(p)) return true;
  if (/\.(pem|p12|pfx|key|kdbx)$/i.test(base)) return true;
  if (/(credentials?|service[-_]?account|private[-_]?key|secrets?)\.(json|ya?ml|toml|ini)$/i.test(base)) return true;
  return false;
}

export function hasShellComposition(command: string) {
  return /[;&|<>`\r\n]/.test(command) || /\$\s*\(/.test(command);
}

function shellWords(command: string) {
  const words: string[] = [];
  let current = "";
  let quote: "'" | '"' | null = null;
  let escaped = false;
  for (const char of command) {
    if (escaped) { current += char; escaped = false; continue; }
    if (char === "\\" && quote !== "'") { escaped = true; continue; }
    if (quote) {
      if (char === quote) quote = null;
      else current += char;
      continue;
    }
    if (char === "'" || char === '"') { quote = char; continue; }
    if (/\s/.test(char)) {
      if (current) { words.push(current); current = ""; }
      continue;
    }
    current += char;
  }
  if (escaped || quote) return null;
  if (current) words.push(current);
  return words;
}

function commandWords(command: string) {
  const words = shellWords(command);
  if (!words) return null;
  let index = 0;
  while (index < words.length && /^[A-Za-z_][A-Za-z0-9_]*=/.test(words[index])) index++;
  if (path.basename(words[index] ?? "").toLowerCase() === "env") {
    index++;
    while (index < words.length && (/^[A-Za-z_][A-Za-z0-9_]*=/.test(words[index]) || words[index].startsWith("-"))) index++;
  }
  return { words, commandIndex: index, executable: path.basename(words[index] ?? "").toLowerCase(), args: words.slice(index + 1) };
}

function isInside(workspaceRoot: string, candidate: string) {
  const root = path.resolve(workspaceRoot);
  const value = path.resolve(candidate);
  return value === root || value.startsWith(root + path.sep);
}

function pathCandidate(token: string) {
  const equals = token.indexOf("=");
  const raw = equals > 0 && token.startsWith("-") ? token.slice(equals + 1) : token;
  if (/^(?:https?|wss?|ssh):\/\//i.test(raw)) return null;
  if (raw === "/dev/null") return null;
  if (raw === "~" || raw.startsWith("~/") || path.isAbsolute(raw) || raw === ".." || raw.startsWith("../") || raw.includes("/../")) return raw;
  return null;
}

function explicitPathEscape(command: string, context: CommandSecurityContext) {
  if (!context.workspaceRoot) return null;
  const parsed = commandWords(command);
  if (!parsed) return null;
  const cwd = path.resolve(context.cwd ?? context.workspaceRoot);
  const rawExecutable = parsed.words[parsed.commandIndex] ?? "";
  const tokens = [rawExecutable, ...parsed.args];
  for (const token of tokens) {
    const candidate = pathCandidate(token);
    if (!candidate) continue;
    const expanded = candidate === "~" ? os.homedir() : candidate.startsWith("~/") ? path.join(os.homedir(), candidate.slice(2)) : path.isAbsolute(candidate) ? candidate : path.resolve(cwd, candidate);
    if (!isInside(context.workspaceRoot, expanded)) return candidate;
  }
  return null;
}

function structuredApproval(command: string) {
  if (hasShellComposition(command)) return null;
  const parsed = commandWords(command);
  if (!parsed?.executable) return null;
  const normalized = command.replace(/\s+/g, " ").trim();
  const redacted = redactCommand(normalized);

  if (parsed.executable === "git") {
    const sub = parsed.args[0]?.toLowerCase();
    const rest = parsed.args.slice(1);
    if (sub === "commit") return { key: "git.commit", label: "Git commit in this workspace" };
    if (sub === "push" && !rest.some((arg) => /^(?:-f|--force(?:-with-lease)?(?:=.*)?|--delete|--mirror|--all|--tags|--prune)$/.test(arg))) {
      const positional = rest.filter((arg) => !arg.startsWith("-"));
      if (positional.some((arg) => arg.startsWith(":"))) return null;
      if (positional[0] && positional[1]) return { key: `git.push:${positional[0]}:${positional[1]}`, label: `Git push ${positional[0]} ${positional[1]}` };
      return null;
    }
    if (["tag", "merge", "rebase", "pull", "fetch"].includes(sub ?? "")) return { key: `git.${sub}:${hashKey(redacted)}`, label: redacted };
  }

  if (/^(?:npm|pnpm|yarn|bun|pip|pipx|poetry|uv|cargo|go)$/i.test(parsed.executable) && /^(?:install|add|remove|uninstall|update|upgrade|get)$/i.test(parsed.args[0] ?? "")) {
    return { key: `dependency:${parsed.executable}:${hashKey(redacted)}`, label: redacted };
  }
  const rawExecutable = parsed.words[parsed.commandIndex] ?? "";
  if (rawExecutable.includes("/") || /^(?:node|python|python3|ruby|perl|php|deno|tsx|ts-node|jest|vitest|pytest|mocha|ava|bash|sh|zsh|fish|pwsh|powershell|java|swift|swiftc|dotnet|xcodebuild)$/i.test(parsed.executable)) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (/^(?:npm|pnpm|yarn|bun)$/i.test(parsed.executable) && /^(?:run|test|start|exec|x)$/i.test(parsed.args[0] ?? "")) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (/^(?:npx|make|just|task|gradle|gradlew|mvn|mvnw)$/i.test(parsed.executable)) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (/^(?:cargo)$/i.test(parsed.executable) && /^(?:run|test|build|bench)$/i.test(parsed.args[0] ?? "")) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (/^(?:go)$/i.test(parsed.executable) && /^(?:run|test|build|generate)$/i.test(parsed.args[0] ?? "")) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (/^(?:flutter|dart)$/i.test(parsed.executable) && /^(?:run|test|build|compile)$/i.test(parsed.args[0] ?? "")) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (parsed.executable === "dotnet" && /^(?:run|test|build|publish)$/i.test(parsed.args[0] ?? "")) {
    return { key: `workspace-exec:${hashKey(redacted)}`, label: redacted };
  }
  if (/^(?:flutter|dart)$/i.test(parsed.executable) && parsed.args[0] === "pub" && /^(?:add|remove|upgrade|downgrade|get)$/i.test(parsed.args[1] ?? "")) {
    return { key: `dependency:${parsed.executable}-pub:${hashKey(redacted)}`, label: redacted };
  }
  if (parsed.executable === "pod" && /^(?:install|update)$/i.test(parsed.args[0] ?? "")) {
    return { key: `dependency:pod:${hashKey(redacted)}`, label: redacted };
  }
  return null;
}

function isRoutineDeveloperCommand(normalized: string) {
  if (hasShellComposition(normalized)) return false;
  return /^(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*(?:(?:fvm\s+)?flutter\s+(?:analyze|test|run|doctor|devices|emulators|clean|build)\b|dart\s+(?:analyze|test|format|fix|compile)\b|xcodebuild\b|xcrun\s+(?:simctl|xctrace)\b|pod\s+(?:repo\s+list|env)\b)/i.test(normalized);
}

export function classifyCommand(command: string, networkPolicy: NetworkPolicy = "approval", context: CommandSecurityContext = {}): PolicyDecision {
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
  hit(/\b(?:security\s+(?:find|dump|unlock|set)|gh\s+auth\s+token|git\s+credential(?:-\w+)?\b)/i, "credential retrieval", "BLOCKED", { block: true });
  hit(/\$(?:\{)?(?:HOME|USERPROFILE|OLDPWD|TMPDIR|XDG_CONFIG_HOME|XDG_DATA_HOME)(?:\})?(?:[\\/]|$)/i, "environment-based path can escape authorized workspace", "BLOCKED", { block: true });
  hit(/^(?:env|printenv|set|export\s+-p)\s*$/i, "environment secret enumeration", "BLOCKED", { block: true });
  if (/\bprintenv\s+([A-Za-z_][A-Za-z0-9_]*)/i.test(normalized)) {
    const name = normalized.match(/\bprintenv\s+([A-Za-z_][A-Za-z0-9_]*)/i)?.[1] ?? "";
    if (SECRET_ENV_NAME.test(name)) hit(/\bprintenv\b/i, "sensitive environment variable access", "BLOCKED", { block: true });
  }
  if (/\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?/.test(normalized)) {
    const vars = [...normalized.matchAll(/\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?/g)].map((m) => m[1]);
    if (vars.some((name) => SECRET_ENV_NAME.test(name))) hit(/\$/i, "sensitive environment variable expansion", "BLOCKED", { block: true });
  }

  const escaped = explicitPathEscape(command, context);
  if (escaped) {
    rules.push(`explicit path escapes authorized workspace: ${redactCommand(escaped)}`);
    risk = "BLOCKED";
    blocked = true;
  } else if (!context.workspaceRoot) {
    hit(/(^|\s)\.\.\/(?:\.\.\/)?/, "parent-directory traversal", "BLOCKED", { block: true });
  }

  hit(/\bgit\s+(?:commit\b[^\n]*--amend\b|reset\s+--hard|clean\s+-[a-z]*f|push\s+[^\n]*(?:-f\b|--force(?:-with-lease)?(?:=\S+)?|--delete\b|--mirror\b|--all\b|--tags\b|--prune\b)|push\s+\S+\s+:\S+|restore\s+(?!--staged))\b/i, "destructive Git action", "CRITICAL", { approve: true });
  hit(/\bgit\s+remote\s+(?:add|remove|rename|set-url)\b/i, "Git remote configuration change", "CRITICAL", { approve: true });
  hit(/\bgit\s+config\b(?![^\n]*\s(?:--get|--get-all|--list|-l)\b)/i, "Git configuration change", "CRITICAL", { approve: true });
  hit(/\brm\s+-[^\s]*r/i, "recursive delete", "CRITICAL", { approve: true });
  hit(/\b(prisma|typeorm|sequelize|knex|alembic|rails)\b[^\n]*(migrate|migration|db:)/i, "database migration", "CRITICAL", { approve: true });
  hit(/\b(?:npm|pnpm|yarn|bun)\s+(?:publish|login|logout)\b/i, "package registry or account action", "CRITICAL", { approve: true });
  hit(/\bdart\s+pub\s+publish\b/i, "package publish action", "CRITICAL", { approve: true });
  hit(/\bpod\s+trunk\s+push\b/i, "package publish action", "CRITICAL", { approve: true });
  hit(/\b(chmod|chown|launchctl|systemctl|service)\b/i, "permission or service modification", "CRITICAL", { approve: true });
  hit(/\b(?:node\s+-e|python(?:3)?\s+-c|ruby\s+-e|perl\s+-e|bash\s+-c|sh\s+-c|zsh\s+-c|fish\s+-c|pwsh\s+-Command|powershell\s+-Command|cmd(?:\.exe)?\s+\/c|osascript\s+-e)\b/i, "inline interpreter can bypass workspace path analysis", "CRITICAL", { approve: true });

  hit(/\bgit\s+(commit|push|tag|merge|rebase|pull|fetch)\b/i, "Git write action", "REVIEW", { approve: true });
  hit(/^(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*(?:(?:\.\/?|[^\s]+\/)[^\s]+|(?:node|python|python3|ruby|perl|php|deno|tsx|ts-node|jest|vitest|pytest|mocha|ava|bash|sh|zsh|fish|pwsh|powershell|java|swift|swiftc|xcodebuild)\b|(?:npm|pnpm|yarn|bun)\s+(?:run|test|start|exec|x)\b|npx\b|(?:make|just|task|gradle|gradlew|mvn|mvnw)\b|cargo\s+(?:run|test|build|bench)\b|go\s+(?:run|test|build|generate)\b|(?:flutter|dart)\s+(?:run|test|build|compile)\b|dotnet\s+(?:run|test|build|publish)\b)/i, "workspace code execution", "REVIEW", { approve: true });
  hit(/\b(npm|pnpm|yarn|bun|pip|pipx|poetry|uv|cargo|go)\s+(install|add|remove|uninstall|update|upgrade|get)\b/i, "dependency or toolchain change", "REVIEW", { approve: true });
  hit(/\b(?:flutter|dart)\s+pub\s+(?:add|remove|upgrade|downgrade|get)\b/i, "Flutter/Dart dependency change", "REVIEW", { approve: true });
  hit(/\bpod\s+(?:install|update|repo\s+update)\b/i, "CocoaPods dependency change", "REVIEW", { approve: true });

  const networkPattern = /\b(curl|wget|ssh|scp|sftp|ftp|nc|ncat|telnet)\b/i;
  if (networkPattern.test(normalized)) {
    if (networkPolicy === "deny") hit(networkPattern, "network access denied by policy", "BLOCKED", { block: true });
    else if (networkPolicy === "approval") hit(networkPattern, "network or remote command", "CRITICAL", { approve: true });
  }

  if (!blocked && hasShellComposition(command)) {
    rules.push("composed shell command requires one-time review");
    if (rank.CRITICAL > rank[risk]) risk = "CRITICAL";
    approval = true;
  }
  if (!blocked && !approval && isRoutineDeveloperCommand(normalized)) rules.push("routine developer command");

  if (blocked) risk = "BLOCKED";
  const uniqueRules = [...new Set(rules)];
  const structured = !blocked && approval && rank[risk] === rank.REVIEW ? structuredApproval(command) : null;
  const approvalPolicy: ApprovalPolicy = blocked ? "blocked" : !approval ? "none" : structured ? "rememberable" : "always";
  const reason = uniqueRules.length ? uniqueRules.join("; ") : "no risky policy rule matched";
  return {
    riskLevel: risk,
    matchedRules: uniqueRules,
    requiresApproval: !blocked && approval,
    blocked,
    redactedCommand: redactCommand(normalized),
    reason,
    approvalPolicy,
    approvalKey: structured?.key,
    approvalLabel: structured?.label,
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
    approvalPolicy: decision.approvalPolicy,
    approvalKey: decision.approvalKey,
    approvalLabel: decision.approvalLabel,
  };
}
