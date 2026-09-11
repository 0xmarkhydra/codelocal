#!/usr/bin/env node
import path from "node:path";
import os from "node:os";
import { promises as fs } from "node:fs";
import { spawn } from "node:child_process";
import { defaultDeviceIdentity, deleteLocalCredential, loadLocalCredential, saveLocalCredential, type LocalDeviceCredential } from "./identity.js";
import { WorkspaceRegistry } from "./workspace-registry.js";
import { ApprovalMemory } from "./approval-memory.js";
import { RuntimeDaemon } from "./runtime-daemon.js";
import { acquireRuntimeLease, runtimeStateDir, runtimeSummary, sendRuntimeCommand, startRuntimeControlServer } from "./runtime-control.js";
import { VERSION } from "./version.js";

const DEFAULT_CLOUD = process.env.CODELOCAL_SERVER ?? "https://codelocal.cloud";

function usage() {
  console.log(`CodeLocal CLI · machine runtime

Quick start:
  cd ~/Projects/my-app
  codelocal .
  codelocal

codelocal . only authorizes the current project locally. codelocal starts the single machine runtime and connects it to CodeLocal Cloud for MCP clients.

Any compatible AI client connected through CodeLocal MCP can list your previously granted workspaces and activate the one you choose for that session.

Commands:
  codelocal                       Start the single machine runtime, or reuse it if already running
  codelocal stop                  Stop the machine runtime
  codelocal --version             Show the installed CodeLocal version
  codelocal grant <project>       Authorize a project folder locally
  codelocal ungrant <id|project>  Remove a project's local authorization
  codelocal workspaces            List authorized local workspaces
  codelocal .                     Add the current project to authorized workspaces, then exit
  codelocal <project-path>        Add a project to authorized workspaces, then exit
  codelocal login                 Open CodeLocal login
  codelocal dashboard             Open CodeLocal dashboard
  codelocal pair [gateway]        Pair this machine manually
  codelocal status                Show pairing + workspace status
  codelocal approvals             List remembered local approvals
  codelocal approvals revoke <id> Revoke one remembered approval
  codelocal approvals reset       Forget all remembered approvals
  codelocal doctor <project>      Run local environment checks
  codelocal mcp ...               Manage local MCP extensions

Default cloud:
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
  if (process.env.CODELOCAL_NO_BROWSER === "1") return false;
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
    const response = await fetch(`${wsToHttp(server)}/api/client/auth/check`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-codelocal-credential-id": credential.credentialId,
        authorization: `Device ${credential.credentialSecret}`,
      },
      body: "{}",
      signal: AbortSignal.timeout(8_000),
    });
    if (response.status === 401 || response.status === 403 || response.status === 404) return false;
    return true;
  } catch {
    return true;
  }
}

async function resolvedRuntime(serverArg?: string, onPhase?: (phase: "checking" | "pairing") => void) {
  // The selected gateway must come from an explicit override or this build's default.
  // Never let a credential saved for an older gateway silently retarget CodeLocal.
  const configured = serverArg || process.env.SERVER_URL || httpToWs(DEFAULT_CLOUD);
  const server = configured.startsWith("ws://") || configured.startsWith("wss://") ? configured : httpToWs(configured);
  onPhase?.("checking");
  let credential = await loadLocalCredential(server);
  if (credential && !(await validateCredential(server, credential))) {
    console.log("Stored CodeLocal credential is no longer valid or the gateway is incompatible. Pairing this machine again…");
    await deleteLocalCredential();
    credential = null;
  }
  if (!credential) {
    onPhase?.("pairing");
    credential = await pair(wsToHttp(server));
  }
  return { server, credential };
}

async function runRuntime(serverArg?: string) {
  const stateDir = runtimeStateDir();
  const lease = await acquireRuntimeLease(VERSION, stateDir);
  if (!lease.acquired) {
    const existing = await runtimeSummary(stateDir);
    console.log("✓ CodeLocal is already running on this machine.");
    if (existing.running && existing.responsive && "detail" in existing && existing.detail && typeof existing.detail === "object") {
      const status = existing.detail as { phase?: string; authorizedWorkspaces?: unknown[]; activeWorkspaces?: unknown[] };
      console.log(`  Status: ${status.phase ?? "online"}`);
      console.log(`  Workspaces: ${status.authorizedWorkspaces?.length ?? "?"} authorized · ${status.activeWorkspaces?.length ?? "?"} active`);
    }
    return;
  }

  type RuntimePhase = "starting" | "checking" | "pairing" | "connecting" | "online" | "stopping";
  let phase: RuntimePhase = "starting";
  let daemon: RuntimeDaemon | null = null;
  let control: Awaited<ReturnType<typeof startRuntimeControlServer>> | null = null;
  let stopping = false;

  const stop = async () => {
    if (stopping) return;
    stopping = true;
    phase = "stopping";
    await daemon?.stop().catch(() => undefined);
    await control?.close().catch(() => undefined);
    await lease.release().catch(() => undefined);
  };
  const stopAndExit = async () => { await stop(); process.exit(0); };

  try {
    control = await startRuntimeControlServer(async (command) => {
      if (command.type === "status") {
        return {
          ...(daemon ? daemon.status() : { authorizedWorkspaces: [], activeWorkspaces: [] }),
          running: true,
          pid: process.pid,
          phase,
        };
      }
      if (command.type === "reload") {
        if (!daemon) return { reloaded: false, pending: true, phase };
        const workspaces = await daemon.syncRegistry(true);
        return { reloaded: true, pending: false, authorizedWorkspaces: workspaces.length, phase };
      }
      if (command.type === "shutdown") {
        setTimeout(() => { void stopAndExit(); }, 25);
        return { stopping: true, phase: "stopping" };
      }
      return null;
    }, stateDir, lease.record.instanceId);

    process.once("SIGINT", () => { void stopAndExit(); });
    process.once("SIGTERM", () => { void stopAndExit(); });

    const { server, credential } = await resolvedRuntime(serverArg, (next) => { phase = next; });
    phase = "connecting";
    daemon = new RuntimeDaemon({
      baseUrl: wsToHttp(server),
      serverUrl: server,
      credential,
      onReady: () => { phase = "online"; },
    });
    await daemon.run();
  } finally {
    await stop();
  }
}

async function grant(projectArg: string, verb = "Granted") {
  if (!projectArg) throw new Error("Usage: codelocal grant <project-folder>");
  const entry = await new WorkspaceRegistry().grant(projectArg);
  console.log(`✓ ${verb} ${entry.workspaceName}`);
  console.log(`  ID: ${entry.workspaceId}`);
  console.log(`  Path: ${entry.localPath}`);
  const reloaded = await sendRuntimeCommand({ type: "reload" }).catch(() => null);
  if (reloaded && typeof reloaded === "object") {
    const result = reloaded as { reloaded?: boolean; pending?: boolean; phase?: string };
    if (result.reloaded) console.log("✓ Running CodeLocal detected; workspace synced.");
    else if (result.pending) console.log(`✓ Running CodeLocal detected (${result.phase ?? "starting"}); workspace queued for sync.`);
    else console.log("✓ Running CodeLocal detected; workspace saved locally.");
  } else {
    const runtime = await runtimeSummary().catch(() => ({ running: false as const }));
    if (runtime.running) console.log("Running CodeLocal detected; workspace saved locally and Cloud sync will retry automatically.");
    else console.log("Run `codelocal` when you want to make this machine available to connected MCP clients.");
  }
  return entry;
}

async function ungrant(identifier: string) {
  if (!identifier) throw new Error("Usage: codelocal ungrant <workspace-id|project-folder>");
  const removed = await new WorkspaceRegistry().revoke(identifier);
  if (!removed) {
    console.log("Workspace was not found in the local authorization registry.");
    return;
  }
  console.log("✓ Workspace authorization removed.");
  const reloaded = await sendRuntimeCommand({ type: "reload" }).catch(() => null);
  if (reloaded && typeof reloaded === "object") {
    const result = reloaded as { reloaded?: boolean; pending?: boolean; phase?: string };
    if (result.reloaded) console.log("✓ Running CodeLocal detected; workspace removal synced.");
    else if (result.pending) console.log(`✓ Running CodeLocal detected (${result.phase ?? "starting"}); removal queued for sync.`);
  } else {
    const runtime = await runtimeSummary().catch(() => ({ running: false as const }));
    if (runtime.running) console.log("Running CodeLocal detected; removal is local now and Cloud sync will retry automatically.");
  }
}

async function listWorkspaces() {
  const workspaces = await new WorkspaceRegistry().list();
  if (!workspaces.length) { console.log("No authorized workspaces. Use `codelocal grant /path/to/project`."); return; }
  for (const workspace of workspaces) {
    console.log(`${workspace.workspaceName}\n  ${workspace.workspaceId}\n  ${workspace.localPath}${workspace.lastActivatedAt ? `\n  last activated ${new Date(workspace.lastActivatedAt).toLocaleString()}` : ""}\n`);
  }
}

async function approvalsCommand(args: string[]) {
  const memory = new ApprovalMemory();
  const action = args[0] ?? "list";
  if (action === "list") {
    const approvals = await memory.list();
    if (!approvals.length) { console.log("No remembered approvals."); return; }
    for (const approval of approvals) {
      console.log(`${approval.label}\n  ID: ${approval.id}\n  Workspace: ${approval.workspaceKey}\n  Key: ${approval.actionKey}\n  Used: ${approval.useCount} · last ${new Date(approval.lastUsedAt).toLocaleString()}\n`);
    }
    return;
  }
  if (action === "revoke") {
    const id = args[1];
    if (!id) throw new Error("Usage: codelocal approvals revoke <id|action-key>");
    const removed = await memory.revoke(id);
    console.log(removed ? `✓ Removed ${removed} remembered approval${removed === 1 ? "" : "s"}.` : "Approval not found.");
    return;
  }
  if (action === "reset") {
    const removed = await memory.reset();
    console.log(`✓ Forgot ${removed} remembered approval${removed === 1 ? "" : "s"}.`);
    return;
  }
  throw new Error("Usage: codelocal approvals [list|revoke <id|action-key>|reset]");
}

async function status() {
  const credential = await loadLocalCredential();
  const workspaces = await new WorkspaceRegistry().list();
  const runtime = await runtimeSummary();
  console.log(JSON.stringify({
    device: defaultDeviceIdentity(),
    paired: !!credential,
    runtime,
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

async function stopRuntime() {
  const stopped = await sendRuntimeCommand({ type: "shutdown" }).catch(() => null);
  console.log(stopped ? "✓ CodeLocal runtime is stopping." : "CodeLocal runtime is not running.");
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
  else if (command === "--version" || command === "-v" || command === "version") console.log(VERSION);
  else if (command === "help" || command === "--help" || command === "-h") usage();
  else if (command === "login") await login(args[0] ?? DEFAULT_CLOUD, false);
  else if (command === "dashboard") await login(args[0] ?? DEFAULT_CLOUD, true);
  else if (command === "pair") { await pair(args[0] ?? DEFAULT_CLOUD); }
  else if (command === "status") await status();
  else if (command === "stop") await stopRuntime();
  else if (command === "approvals") await approvalsCommand(args);
  else if (command === "workspaces") await listWorkspaces();
  else if (command === "grant") await grant(args[0]);
  else if (command === "ungrant") await ungrant(args[0]);
  else if (command === "start") {
    if (args[0] && await looksLikeProjectPath(args[0])) {
      await grant(args[0]);
      await runRuntime(args[1]);
    } else await runRuntime(args[0]);
  }
  else if (command === "doctor" || command === "mcp") await delegateLegacyCli();
  else if (command === "rotate" || command === "revoke") {
    console.log("Device rotation/revocation is account-scoped in SaaS mode. Opening Security/Devices dashboard…");
    await login(DEFAULT_CLOUD, true);
  }
  else if (await looksLikeProjectPath(command)) await grant(command, "Added");
  else { usage(); process.exitCode = 1; }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
