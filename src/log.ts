type LogLevel = "debug" | "info" | "warn" | "error";
type TerminalTone = "success" | "info" | "accent" | "warn" | "error" | "muted";

const LOG_LEVEL = (process.env.CODELOCAL_LOG_LEVEL ?? "info").toLowerCase();
const LOG_FORMAT = (process.env.CODELOCAL_LOG_FORMAT ?? "json").toLowerCase();
const ranks: Record<LogLevel, number> = { debug: 10, info: 20, warn: 30, error: 40 };
const configuredRank = ranks[(LOG_LEVEL in ranks ? LOG_LEVEL : "info") as LogLevel];

const ansi = {
  reset: "\u001b[0m",
  bold: "\u001b[1m",
  dim: "\u001b[2m",
  gray: "\u001b[90m",
  cyan: "\u001b[36m",
  green: "\u001b[32m",
  yellow: "\u001b[33m",
  red: "\u001b[31m",
  magenta: "\u001b[35m",
};

function colorEnabled() {
  return process.env.NO_COLOR == null && process.env.TERM !== "dumb" && !!process.stdout.isTTY;
}

function paint(code: string, value: unknown) {
  const text = String(value ?? "");
  return colorEnabled() ? `${code}${text}${ansi.reset}` : text;
}

function bold(value: unknown) { return paint(ansi.bold, value); }
function dim(value: unknown) { return paint(ansi.gray, value); }

function redactText(input: string) {
  let value = input;
  value = value.replace(/(authorization\s*:\s*bearer\s+)[^\s\"']+/gi, "$1[REDACTED]");
  value = value.replace(/((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[=:]\s*)[^\s\"']+/gi, "$1[REDACTED]");
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

function duration(value: unknown) {
  const ms = Number(value);
  if (!Number.isFinite(ms)) return "";
  if (ms < 1000) return `${Math.max(0, Math.round(ms))}ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`;
}

function compact(value: unknown, max = 76) {
  const text = redactText(String(value ?? "")).replace(/\s+/g, " ").trim();
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

function toolDetail(args: unknown) {
  if (!args || typeof args !== "object") return "";
  const value = args as Record<string, unknown>;
  if (typeof value.command === "string") return `$ ${compact(value.command, 64)}`;
  if (typeof value.path === "string") {
    const lines = value.startLine != null ? `  L${value.startLine}${value.endLine != null ? `–${value.endLine}` : ""}` : "";
    return `${compact(value.path, 58)}${lines}`;
  }
  if (Array.isArray(value.paths)) return `${value.paths.length} file${value.paths.length === 1 ? "" : "s"}`;
  if (typeof value.query === "string") return `“${compact(value.query, 58)}”`;
  if (typeof value.name === "string") return compact(value.name, 58);
  if (typeof value.processId === "string") return `process ${value.processId.slice(0, 8)}`;
  if (typeof value.cwd === "string" && value.cwd !== ".") return `in ${compact(value.cwd, 58)}`;
  return "";
}

function terminalIcon(tone: TerminalTone) {
  if (tone === "success") return paint(ansi.green, "●");
  if (tone === "accent") return paint(ansi.magenta, "◆");
  if (tone === "warn") return paint(ansi.yellow, "▲");
  if (tone === "error") return paint(ansi.red, "✕");
  if (tone === "muted") return paint(ansi.gray, "◌");
  return paint(ansi.cyan, "◇");
}

export function terminalStatus(tone: TerminalTone, label: string, detail = "") {
  const padded = `${label}`.padEnd(12, " ");
  console.log(`  ${terminalIcon(tone)} ${bold(padded)} ${detail}`.trimEnd());
}

export function terminalHeader(title = "CodeLocal", subtitle = "Local runtime for ChatGPT") {
  const width = 48;
  const top = `╭${"─".repeat(width - 2)}╮`;
  const bottom = `╰${"─".repeat(width - 2)}╯`;
  const row = (text: string) => `│  ${text}${" ".repeat(Math.max(0, width - 4 - text.length))}│`;
  console.log("");
  console.log(paint(ansi.magenta, top));
  console.log(paint(ansi.magenta, row(`◆ ${title}`)));
  console.log(paint(ansi.gray, row(subtitle)));
  console.log(paint(ansi.magenta, bottom));
  console.log("");
}

export function mirrorProcessOutput(stream: "stdout" | "stderr", text: string) {
  if (LOG_FORMAT !== "pretty") {
    process.stdout.write(text);
    return;
  }
  const marker = stream === "stderr" ? paint(ansi.yellow, "│") : paint(ansi.gray, "│");
  const lines = text.replace(/\r/g, "").split("\n");
  const trailingNewline = text.endsWith("\n");
  const rendered = lines
    .filter((line, index) => line.length > 0 || index < lines.length - 1)
    .map((line) => `     ${marker} ${line}`)
    .join("\n");
  if (rendered) process.stdout.write(rendered + (trailingNewline ? "\n" : ""));
}

function prettyLog(level: LogLevel, event: string, fields: Record<string, unknown>) {
  const tool = compact(fields.tool ?? "", 42);
  const elapsed = duration(fields.durationMs);

  if (event === "workspace.watcher_configured") {
    terminalStatus("info", "Watcher", `${fields.backend ?? "native"} ${dim(`· fallback ${fields.fallbackBackend ?? "bounded"}`)}`);
    return;
  }
  if (event === "mcp.cloud_sync") {
    const reason = fields.reason ? ` · ${fields.reason}` : "";
    terminalStatus(fields.skipped ? "muted" : "success", "MCP sync", `${fields.skipped ? "Skipped" : "Ready"}${dim(reason)}`);
    return;
  }
  if (event === "mcp.cloud_sync_failed") {
    terminalStatus("warn", "MCP sync", compact(fields.error ?? "Cloud sync unavailable"));
    return;
  }
  if (event === "client.started") {
    terminalStatus("accent", "Workspace", `${fields.workspaceId ?? "workspace"} ${dim(`· approval ${fields.approvalMode ?? "prompt"}`)}`);
    return;
  }
  if (event === "client.connecting") {
    terminalStatus("info", "Cloud", "Connecting…");
    return;
  }
  if (event === "client.registered") {
    terminalStatus("success", "Cloud", `Connected ${dim(`· protocol v${fields.protocolVersion ?? "?"}`)}`);
    console.log("");
    terminalStatus("muted", "Status", "Waiting for ChatGPT…");
    return;
  }
  if (event === "client.disconnected") {
    terminalStatus("warn", "Cloud", `Disconnected ${dim(`· retry ${duration(fields.reconnectInMs)}`)}`);
    return;
  }
  if (event === "client.socket_error") {
    const error = fields.error as Record<string, unknown> | undefined;
    terminalStatus("error", "Cloud", compact(error?.message ?? "Connection error"));
    return;
  }
  if (event === "tool.received") {
    const detail = toolDetail(fields.args);
    console.log(`  ${paint(ansi.magenta, "◆")} ${paint(ansi.cyan, bold(tool || "tool"))}${detail ? `  ${dim(detail)}` : ""}`);
    return;
  }
  if (event === "tool.completed") {
    console.log(`  ${paint(ansi.green, "✓")} ${bold(tool || "tool")}  ${dim(elapsed || "done")}`);
    return;
  }
  if (event === "tool.failed") {
    const error = fields.error as Record<string, unknown> | undefined;
    console.error(`  ${paint(ansi.red, "✕")} ${bold(tool || "tool")}  ${dim(elapsed)}${error?.message ? `  ${paint(ansi.red, compact(error.message, 72))}` : ""}`);
    return;
  }

  const detail = Object.entries(fields)
    .filter(([key]) => !["requestId", "tool"].includes(key))
    .slice(0, 3)
    .map(([key, value]) => `${key}=${compact(typeof value === "object" ? JSON.stringify(value) : value, 38)}`)
    .join("  ");
  const tone: TerminalTone = level === "error" ? "error" : level === "warn" ? "warn" : level === "debug" ? "muted" : "info";
  terminalStatus(tone, event.replace(/^client\.|^workspace\./, ""), detail);
}

export function log(level: LogLevel, event: string, fields: Record<string, unknown> = {}) {
  if (ranks[level] < configuredRank) return;
  const cleanFields = sanitize(fields) as Record<string, unknown>;
  if (LOG_FORMAT === "pretty") {
    prettyLog(level, event, cleanFields);
    return;
  }
  const entry = {
    ts: new Date().toISOString(),
    level,
    event,
    ...cleanFields,
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
