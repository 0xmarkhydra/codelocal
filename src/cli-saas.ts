#!/usr/bin/env node
import path from "node:path";
import os from "node:os";
import { promises as fs } from "node:fs";
import { spawn } from "node:child_process";
import { defaultDeviceIdentity, deleteLocalCredential, loadLocalCredential, saveLocalCredential, type LocalDeviceCredential } from "./identity.js";
import { WorkspaceRegistry } from "./workspace-registry.js";
import { RuntimeDaemon } from "./runtime-daemon.js";

const DEFAULT_CLOUD = process.env.CODELOCAL_SERVER ?? "https://codelocal-mcp-dev-dev.up.railway.app";

function usage() {
  console.log(`CodeLocal CLI · machine runtime

Quick start:
  codelocal

One-time workspace access:
  codelocal grant ~/Projects/my-app

Then keep only this running:
  codelocal

ChatGPT can list your previously granted workspaces and activate the one you choose in chat.

Commands:
  codelocal                       Start the machine runtime; no project cwd required
  codelocal grant <project>       Authorize a project folder locally
  codelocal ungrant <id|project>  Remove a project's local authorization
  codelocal workspaces            List authorized local workspaces
  codelocal .                     Backward-compatible: grant + activate current project
  codelocal <project-path>        Backward-compatible: grant + activate a project
  codelocal login                 Open CodeLocal login
  codelocal dashboard             Open CodeLocal dashboard
  codelocal pair [gateway]        Pair this machine manually
  codelocal status                Show pairing + workspace status
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
  throw new Error("Pairing expired. Run `codelocal` again to create a new pairing request.");
}

async function validateCredential(server: string, credential: LocalDeviceCredential) {
  try {
    const response = await fetch(`${wsToHttp(server)}/api/client/mcp-sync`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-codelocal-credential-id": credential.credentialId,
        authorization: `Device ${credential.credentialSecret}`,
      },
      body: JSON.stringify({ workspaceId: "" }),
      signal: AbortSignal.timeout(8_000),
    });
    if (response.status === 401 || response.status === 403) return false;
    return true;
  } catch {
    return true;
  }
}

async function resolvedRuntime(serverArg?: string) {
  const anyCredential = await loadLocalCredential();
  const configured = serverArg || process.env.SERVER_URL || anyCredential?.serverUrl || httpToWs(DEFAULT_CLOUD);
  const server = configured.startsWith("ws://") || configured.startsWith("wss://") ? configured : httpToWs(configured);
  let credential = await loadLocalCredential(server);
  if (credential && !(await validateCredential(server, credential))) {
    console.log("Stored CodeLocal credential is no longer valid. Pairing this machine again…");
    await deleteLocalCredential();
    credential = null;
  }
  if (!credential) credential = await pair(wsToHttp(server));
  return { server, credential };
}

async function runRuntime(projectArg?: string, serverArg?: string) {
  const registry = new WorkspaceRegistry();
  let initialWorkspaceId: string | undefined;
  if (projectArg) {
    const granted = await registry.grant(projectArg);
    initialWorkspaceId = granted.workspaceId;
    console.log(`✓ Workspace granted: ${granted.workspaceName}`);
  }
  const { server, credential } = await resolvedRuntime(serverArg);
  const daemon = new RuntimeDaemon({ baseUrl: wsToHttp(server), serverUrl: server, credential, initialWorkspaceId });
  const stop = async () => { await daemon.stop(); process.exit(0); };
  process.once("SIGINT", () => { void stop(); });
  process.once("SIGTERM", () => { void stop(); });
  await daemon.run();
}

async function grant(projectArg: string) {
  if (!projectArg) throw new Error("Usage: codelocal grant <project-folder>");
  const entry = await new WorkspaceRegistry().grant(projectArg);
  console.log(`✓ Granted ${entry.workspaceName}`);
  console.log(`  ID: ${entry.workspaceId}`);
  console.log(`  Path: ${entry.localPath}`);
  console.log("If `codelocal` is already running, it will sync this workspace shortly.");
}

async function ungrant(identifier: string) {
  if (!identifier) throw new Error("Usage: codelocal ungrant <workspace-id|project-folder>");
  const removed = await new WorkspaceRegistry().revoke(identifier);
  console.log(removed ? "✓ Workspace authorization removed." : "Workspace was not found in the local authorization registry.");
}

async function listWorkspaces() {
  const workspaces = await new WorkspaceRegistry().list();
  if (!workspaces.length) { console.log("No authorized workspaces. Use `codelocal grant /path/to/project`."); return; }
  for (const workspace of workspaces) {
    console.log(`${workspace.workspaceName}\n  ${workspace.workspaceId}\n  ${workspace.localPath}${workspace.lastActivatedAt ? `\n  last activated ${new Date(workspace.lastActivatedAt).toLocaleString()}` : ""}\n`);
  }
}

async function status() {
  const credential = await loadLocalCredential();
  const workspaces = await new WorkspaceRegistry().list();
  console.log(JSON.stringify({
    device: defaultDeviceIdentity(),
    paired: !!credential,
    authorizedWorkspaces: workspaces.map(({ workspaceId, workspaceName, localPath, grantedAt, lastActivatedAt }) => ({ workspaceId, workspaceName, localPath, grantedAt, lastActivatedAt })),
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
  if (!command) await runRuntime();
  else if (command === "help" || command === "--help" || command === "-h") usage();
  else if (command === "login") await login(args[0] ?? DEFAULT_CLOUD, false);
  else if (command === "dashboard") await login(args[0] ?? DEFAULT_CLOUD, true);
  else if (command === "pair") { await pair(args[0] ?? DEFAULT_CLOUD); }
  else if (command === "status") await status();
  else if (command === "workspaces") await listWorkspaces();
  else if (command === "grant") await grant(args[0]);
  else if (command === "ungrant") await ungrant(args[0]);
  else if (command === "start") await runRuntime(args[0] ?? ".", args[1]);
  else if (command === "doctor" || command === "mcp") await delegateLegacyCli();
  else if (command === "rotate" || command === "revoke") {
    console.log("Device rotation/revocation is account-scoped in SaaS mode. Opening Security/Devices dashboard…");
    await login(DEFAULT_CLOUD, true);
  }
  else if (await looksLikeProjectPath(command)) await runRuntime(command, args[0]);
  else { usage(); process.exitCode = 1; }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
