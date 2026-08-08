#!/usr/bin/env node
import path from "node:path";
import os from "node:os";
import { createHash } from "node:crypto";
import { promises as fs } from "node:fs";
import { spawn } from "node:child_process";
import { defaultDeviceIdentity, loadLocalCredential, saveLocalCredential } from "./identity.js";

const DEFAULT_CLOUD = process.env.CODELOCAL_SERVER ?? "https://codelocal-mcp-dev-dev.up.railway.app";

function usage() {
  console.log(`CodeLocal CLI · SaaS MVP

Quick start:
  cd ~/your-project
  codelocal .

Commands:
  codelocal .                     Connect current project
  codelocal <project-path>        Connect another project
  codelocal login                 Open CodeLocal login
  codelocal dashboard             Open CodeLocal dashboard
  codelocal pair [gateway]        Pair this machine manually
  codelocal status                Show local pairing status
  codelocal doctor <project>      Run local environment checks
  codelocal mcp ...               Manage local MCP extensions

Default dev cloud:
  ${DEFAULT_CLOUD}
`);
}

function normalizeBase(value: string) {
  const url = new URL(value);
  if (url.protocol === "ws:") url.protocol = "http:";
  if (url.protocol === "wss:") url.protocol = "https:";
  url.pathname = ""; url.search = ""; url.hash = "";
  return url.toString().replace(/\/$/, "");
}

function httpToWs(base: string) {
  const url = new URL(normalizeBase(base));
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = "/client";
  return url.toString();
}

function wsToHttp(value: string) {
  return normalizeBase(value);
}

function openBrowser(url: string) {
  try {
    let command: string;
    let args: string[];
    if (process.platform === "darwin") { command = "open"; args = [url]; }
    else if (process.platform === "win32") { command = "cmd"; args = ["/c", "start", "", url]; }
    else { command = "xdg-open"; args = [url]; }
    const child = spawn(command, args, { detached: true, stdio: "ignore", shell: false });
    child.unref();
    return true;
  } catch { return false; }
}

async function pair(baseArg = DEFAULT_CLOUD) {
  const base = normalizeBase(baseArg || DEFAULT_CLOUD);
  const wsUrl = httpToWs(base);
  const existing = await loadLocalCredential(wsUrl);
  if (existing) return existing;

  const device = defaultDeviceIdentity();
  const response = await fetch(`${base}/pair/start`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(device),
    signal: AbortSignal.timeout(15_000),
  });
  if (!response.ok) throw new Error(`Unable to start device pairing (${response.status}).`);
  const pairing = await response.json() as { pairingId: string; code: string; expiresAt: number; approveUrl: string };

  console.log(`\nCodeLocal needs to pair this machine.\n`);
  console.log(`Pairing code: ${pairing.code}`);
  console.log(`Approve: ${pairing.approveUrl}\n`);
  if (openBrowser(pairing.approveUrl)) console.log("Opened your default browser. Sign in to CodeLocal and approve this device.");
  else console.log("Open the approval URL in your browser, sign in, and approve this device.");
  console.log("Waiting for approval…");

  while (Date.now() < pairing.expiresAt) {
    await new Promise((resolve) => setTimeout(resolve, 1800));
    const claim = await fetch(`${base}/pair/claim`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ pairingId: pairing.pairingId, code: pairing.code }),
      signal: AbortSignal.timeout(10_000),
    }).catch(() => null);
    if (!claim?.ok) continue;
    const credential = await claim.json() as { credentialId: string; credentialSecret: string; deviceId: string; deviceName: string };
    const saved = await saveLocalCredential({ ...credential, serverUrl: wsUrl });
    console.log(`✓ ${credential.deviceName} paired with CodeLocal Cloud.\n`);
    return saved;
  }
  throw new Error("Pairing expired. Run `codelocal .` again to create a new pairing request.");
}

function defaultWorkspaceId(project: string) {
  const slug = path.basename(project).replace(/[^A-Za-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 48) || "workspace";
  const digest = createHash("sha256").update(project).digest("hex").slice(0, 10);
  return `${slug}-${digest}`;
}

async function start(projectArg: string, serverArg?: string) {
  const project = await fs.realpath(path.resolve(projectArg || "."));
  const anyCredential = await loadLocalCredential();
  const configured = serverArg || process.env.SERVER_URL || anyCredential?.serverUrl || httpToWs(DEFAULT_CLOUD);
  const server = configured.startsWith("ws://") || configured.startsWith("wss://") ? configured : httpToWs(configured);
  let credential = await loadLocalCredential(server);
  if (!credential) credential = await pair(wsToHttp(server));

  process.env.PROJECT_ROOT = project;
  process.env.SERVER_URL = server;
  process.env.CODELOCAL_WORKSPACE_ID ??= defaultWorkspaceId(project);
  process.env.CODELOCAL_WORKSPACE_NAME ??= path.basename(project);
  process.env.CODELOCAL_ALLOW_SHELL ??= "1";
  process.env.CODELOCAL_APPROVAL_MODE ??= "prompt";

  console.log(`CodeLocal workspace: ${project}`);
  console.log(`Workspace ID: ${process.env.CODELOCAL_WORKSPACE_ID}`);
  console.log(`Cloud: ${wsToHttp(server)}`);
  await import("./client-entry-v2.js");
}

async function status() {
  const credential = await loadLocalCredential();
  console.log(JSON.stringify({
    device: defaultDeviceIdentity(),
    paired: !!credential,
    credential: credential ? {
      credentialId: credential.credentialId,
      deviceId: credential.deviceId,
      deviceName: credential.deviceName,
      serverUrl: credential.serverUrl,
      createdAt: credential.createdAt,
      credentialSecret: "[REDACTED]",
    } : null,
    stateDir: process.env.CODELOCAL_STATE_DIR ?? path.join(os.homedir(), ".codelocal"),
  }, null, 2));
}

async function login(baseArg = DEFAULT_CLOUD, dashboard = false) {
  const base = normalizeBase(baseArg || DEFAULT_CLOUD);
  const url = dashboard ? `${base}/dashboard` : `${base}/login`;
  console.log(url);
  if (!openBrowser(url)) console.log("Open the URL above in your browser.");
}

async function looksLikeProjectPath(value: string) {
  if (!value || value.startsWith("-")) return false;
  if (value === "." || value === ".." || value.startsWith("./") || value.startsWith("../") || path.isAbsolute(value)) return true;
  return fs.stat(path.resolve(value)).then((stat) => stat.isDirectory()).catch(() => false);
}

async function delegateLegacyCli() {
  await import("./cli.js");
}

const [, , command, ...args] = process.argv;
try {
  if (!command || command === "help" || command === "--help" || command === "-h") usage();
  else if (command === "login") await login(args[0] ?? DEFAULT_CLOUD, false);
  else if (command === "dashboard") await login(args[0] ?? DEFAULT_CLOUD, true);
  else if (command === "pair") { await pair(args[0] ?? DEFAULT_CLOUD); }
  else if (command === "status") await status();
  else if (command === "start") await start(args[0] ?? ".", args[1]);
  else if (command === "doctor" || command === "mcp") await delegateLegacyCli();
  else if (command === "rotate" || command === "revoke") {
    console.log("Device rotation/revocation is account-scoped in SaaS mode. Opening Security/Devices dashboard…");
    await login(DEFAULT_CLOUD, true);
  }
  else if (await looksLikeProjectPath(command)) await start(command, args[0]);
  else { usage(); process.exitCode = 1; }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
