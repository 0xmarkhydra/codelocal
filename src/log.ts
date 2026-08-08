type LogLevel = "debug" | "info" | "warn" | "error";

const LOG_LEVEL = (process.env.CODELOCAL_LOG_LEVEL ?? "info").toLowerCase();
const ranks: Record<LogLevel, number> = { debug: 10, info: 20, warn: 30, error: 40 };
const configuredRank = ranks[(LOG_LEVEL in ranks ? LOG_LEVEL : "info") as LogLevel];

function redactText(input: string) {
  let value = input;
  value = value.replace(/(authorization\s*:\s*bearer\s+)[^\s"']+/gi, "$1[REDACTED]");
  value = value.replace(/((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[=:]\s*)[^\s"']+/gi, "$1[REDACTED]");
  value = value.replace(/(--(?:token|password|secret|api-key|apikey)\s+)([^\s]+)/gi, "$1[REDACTED]");
  value = value.replace(/((?:OPENAI_API_KEY|CODEX_API_KEY|AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|GH_TOKEN)=)([^\s]+)/gi, "$1[REDACTED]");
  value = value.replace(/(https?:\/\/[^\s:@/]+:)[^\s@/]+@/gi, "$1[REDACTED]@");
  return value;
}

function sanitize(value: unknown): unknown {
  if (value instanceof Error) return { name: value.name, message: redactText(value.message) };
  if (Array.isArray(value)) return value.map(sanitize);
  if (!value || typeof value !== "object") return typeof value === "string" ? redactText(value) : value;
  const out: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (/token|secret|password|authorization|cookie/i.test(key)) out[key] = "[redacted]";
    else if (/content|patch|oldText|newText|input/i.test(key) && typeof item === "string") out[key] = `[${Buffer.byteLength(item, "utf8")} bytes]`;
    else if (typeof item === "string") out[key] = redactText(item);
    else out[key] = sanitize(item);
  }
  return out;
}

export function log(level: LogLevel, event: string, fields: Record<string, unknown> = {}) {
  if (ranks[level] < configuredRank) return;
  const entry = {
    ts: new Date().toISOString(),
    level,
    event,
    ...sanitize(fields) as Record<string, unknown>,
  };
  const line = JSON.stringify(entry);
  if (level === "error") console.error(line);
  else if (level === "warn") console.warn(line);
  else console.log(line);
}

export function summarizeToolArgs(tool: string, args: unknown) {
  if (!args || typeof args !== "object") return {};
  const source = args as Record<string, unknown>;
  const summary: Record<string, unknown> = {};
  for (const key of ["path", "paths", "cwd", "processId", "cursor", "cached", "maxDepth", "maxResults", "fixedStrings", "yieldMs", "timeoutMs", "signal", "includeIgnored", "expectedHash", "startLine", "endLine", "limit", "name", "ref"]) {
    if (key in source) summary[key] = source[key];
  }
  if (typeof source.command === "string") summary.command = redactText(source.command.slice(0, 500));
  if (typeof source.query === "string") summary.query = redactText(source.query.slice(0, 300));
  if (typeof source.content === "string") summary.contentBytes = Buffer.byteLength(source.content, "utf8");
  if (typeof source.patch === "string") summary.patchBytes = Buffer.byteLength(source.patch, "utf8");
  if (typeof source.input === "string") summary.inputBytes = Buffer.byteLength(source.input, "utf8");
  return summary;
}
