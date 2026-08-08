type LogLevel = "debug" | "info" | "warn" | "error";

const LOG_LEVEL = (process.env.CODELOCAL_LOG_LEVEL ?? "info").toLowerCase();
const ranks: Record<LogLevel, number> = { debug: 10, info: 20, warn: 30, error: 40 };
const configuredRank = ranks[(LOG_LEVEL in ranks ? LOG_LEVEL : "info") as LogLevel];

function sanitize(value: unknown): unknown {
  if (value instanceof Error) return { name: value.name, message: value.message };
  if (Array.isArray(value)) return value.map(sanitize);
  if (!value || typeof value !== "object") return value;
  const out: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (/token|secret|password|authorization|cookie/i.test(key)) out[key] = "[redacted]";
    else if (/content|patch|oldText|newText|input/i.test(key) && typeof item === "string") out[key] = `[${Buffer.byteLength(item, "utf8")} bytes]`;
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
  for (const key of ["path", "paths", "cwd", "processId", "cursor", "cached", "maxDepth", "maxResults", "fixedStrings", "yieldMs", "timeoutMs", "signal"]) {
    if (key in source) summary[key] = source[key];
  }
  if (typeof source.command === "string") summary.command = source.command.slice(0, 500);
  if (typeof source.query === "string") summary.query = source.query.slice(0, 300);
  if (typeof source.content === "string") summary.contentBytes = Buffer.byteLength(source.content, "utf8");
  if (typeof source.patch === "string") summary.patchBytes = Buffer.byteLength(source.patch, "utf8");
  if (typeof source.input === "string") summary.inputBytes = Buffer.byteLength(source.input, "utf8");
  return summary;
}
