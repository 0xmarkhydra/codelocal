export type ClientUpdateLevel = "recommended" | "required";

export type ClientReleaseManifest = {
  latestVersion: string | null;
  minimumVersion: string | null;
  channel: string;
  updateCommand: string;
  restartCommand: string;
  message: string;
};

export type ClientUpdateNotice = {
  key: string;
  level: ClientUpdateLevel;
  installedVersion: string | null;
  latestVersion: string;
  minimumVersion: string | null;
  channel: string;
  updateCommand: string;
  restartCommand: string;
  message: string;
};

type ParsedSemver = {
  major: number;
  minor: number;
  patch: number;
  prerelease: string[];
};

function normalizeVersion(value: unknown) {
  if (typeof value !== "string") return null;
  const normalized = value.trim().replace(/^v(?=\d)/i, "");
  return normalized || null;
}

function parseSemver(value: unknown): ParsedSemver | null {
  const normalized = normalizeVersion(value);
  if (!normalized) return null;
  const match = /^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/.exec(normalized);
  if (!match) return null;
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    prerelease: match[4] ? match[4].split(".") : [],
  };
}

function comparePrerelease(left: string[], right: string[]) {
  if (!left.length && !right.length) return 0;
  if (!left.length) return 1;
  if (!right.length) return -1;
  const length = Math.max(left.length, right.length);
  for (let i = 0; i < length; i++) {
    const a = left[i];
    const b = right[i];
    if (a == null) return -1;
    if (b == null) return 1;
    if (a === b) continue;
    const aNumeric = /^\d+$/.test(a);
    const bNumeric = /^\d+$/.test(b);
    if (aNumeric && bNumeric) return Number(a) < Number(b) ? -1 : 1;
    if (aNumeric !== bNumeric) return aNumeric ? -1 : 1;
    return a < b ? -1 : 1;
  }
  return 0;
}

export function compareSemver(left: unknown, right: unknown): number | null {
  const a = parseSemver(left);
  const b = parseSemver(right);
  if (!a || !b) return null;
  for (const field of ["major", "minor", "patch"] as const) {
    if (a[field] !== b[field]) return a[field] < b[field] ? -1 : 1;
  }
  return comparePrerelease(a.prerelease, b.prerelease);
}

function releaseChannel(value: unknown) {
  const channel = typeof value === "string" ? value.trim() : "";
  return /^[A-Za-z][A-Za-z0-9._-]{0,31}$/.test(channel) ? channel : "beta";
}

export function clientReleaseManifest(env: NodeJS.ProcessEnv = process.env): ClientReleaseManifest {
  const channel = releaseChannel(env.CODELOCAL_RELEASE_CHANNEL ?? "beta");
  const defaultUpdateCommand = `npm i -g codelocal@${channel}`;
  return {
    latestVersion: normalizeVersion(env.CODELOCAL_LATEST_CLIENT_VERSION),
    minimumVersion: normalizeVersion(env.CODELOCAL_MIN_CLIENT_VERSION),
    channel,
    updateCommand: String(env.CODELOCAL_UPDATE_COMMAND ?? defaultUpdateCommand).trim() || defaultUpdateCommand,
    restartCommand: String(env.CODELOCAL_RESTART_COMMAND ?? "codelocal").trim() || "codelocal",
    message: String(env.CODELOCAL_UPDATE_MESSAGE ?? "CodeLocal has a new version available. Please tell the user to update.").trim(),
  };
}

export function evaluateClientUpdate(installedVersion: unknown, manifest: ClientReleaseManifest): ClientUpdateNotice | null {
  const latest = normalizeVersion(manifest.latestVersion);
  if (!latest || !parseSemver(latest)) return null;

  const installed = normalizeVersion(installedVersion);
  const comparison = installed ? compareSemver(installed, latest) : null;
  if (comparison != null && comparison >= 0) return null;

  let level: ClientUpdateLevel = "recommended";
  const minimum = normalizeVersion(manifest.minimumVersion);
  if (installed && minimum && parseSemver(minimum)) {
    const minimumComparison = compareSemver(installed, minimum);
    if (minimumComparison != null && minimumComparison < 0) level = "required";
  }

  return {
    key: `client-update:${latest}`,
    level,
    installedVersion: installed,
    latestVersion: latest,
    minimumVersion: minimum,
    channel: manifest.channel,
    updateCommand: manifest.updateCommand,
    restartCommand: manifest.restartCommand,
    message: manifest.message,
  };
}

export function renderClientUpdateNotice(notice: ClientUpdateNotice) {
  const installed = notice.installedVersion ?? "unknown (legacy client)";
  const urgency = notice.level === "required"
    ? "This client is below the configured minimum version. Ask the user to update now."
    : "Ask the user to update, while still answering their current request normally.";
  return [
    "[CODELOCAL_UPDATE_NOTICE]",
    "IMPORTANT FOR CHATGPT: Surface this CodeLocal update notice to the user in your final response.",
    notice.message,
    `Update level: ${notice.level}`,
    `Installed: ${installed}`,
    `Latest: ${notice.latestVersion}`,
    notice.minimumVersion ? `Minimum supported client: ${notice.minimumVersion}` : null,
    `Update command: ${notice.updateCommand}`,
    `Then run: ${notice.restartCommand}`,
    "Pairing and existing workspace grants do not need to be recreated after a normal update.",
    urgency,
    "Do not repeat this notice again in the same ChatGPT MCP session for this release.",
    "[/CODELOCAL_UPDATE_NOTICE]",
  ].filter(Boolean).join("\n");
}
