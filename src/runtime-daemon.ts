import { spawn, type ChildProcess } from "node:child_process";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";
import type { LocalDeviceCredential } from "./identity.js";
import { terminalHeader, terminalStatus } from "./log.js";
import { WorkspaceRegistry, type AuthorizedWorkspace } from "./workspace-registry.js";
import { VERSION } from "./version.js";

export type RuntimeDaemonOptions = {
  baseUrl: string;
  serverUrl: string;
  credential: LocalDeviceCredential;
  pollMs?: number;
};

type ActivationResponse = {
  activation?: { workspaceId: string; requestId: string; requestedAt: number } | null;
  revocation?: { workspaceId: string; requestId: string; requestedAt: number } | null;
};

function deviceHeaders(credential: LocalDeviceCredential) {
  return {
    "content-type": "application/json",
    "x-codelocal-credential-id": credential.credentialId,
    authorization: `Device ${credential.credentialSecret}`,
  };
}

function registrySignature(workspaces: AuthorizedWorkspace[]) {
  return JSON.stringify(workspaces.map((workspace) => [workspace.workspaceId, workspace.workspaceName]).sort((a, b) => String(a[0]).localeCompare(String(b[0]))));
}

export class RuntimeDaemon {
  private registry = new WorkspaceRegistry();
  private children = new Map<string, ChildProcess>();
  private stopped = false;
  private workspaces: AuthorizedWorkspace[] = [];
  private syncedSignature = "";

  constructor(private options: RuntimeDaemonOptions) {}

  private stopUnauthorizedChildren(workspaces: AuthorizedWorkspace[]) {
    const allowed = new Set(workspaces.map((workspace) => workspace.workspaceId));
    for (const [workspaceId, child] of this.children) {
      if (allowed.has(workspaceId)) continue;
      if (child.exitCode == null && !child.killed) child.kill("SIGTERM");
      this.children.delete(workspaceId);
      terminalStatus("warn", "Workspace", `${workspaceId} deactivated · authorization removed`);
    }
  }

  async syncRegistry(force = false) {
    const workspaces = await this.registry.list();
    this.stopUnauthorizedChildren(workspaces);
    const signature = registrySignature(workspaces);
    this.workspaces = workspaces;
    if (!force && signature === this.syncedSignature) return workspaces;
    const response = await fetch(`${this.options.baseUrl}/api/client/workspaces/sync`, {
      method: "POST",
      headers: deviceHeaders(this.options.credential),
      body: JSON.stringify({ clientVersion: VERSION, workspaces: workspaces.map(({ workspaceId, workspaceName, grantedAt, lastActivatedAt }) => ({ workspaceId, workspaceName, grantedAt, lastActivatedAt })) }),
      signal: AbortSignal.timeout(10_000),
    });
    if (response.status === 401 || response.status === 403) throw new Error("Stored CodeLocal device credential is no longer valid.");
    if (!response.ok) throw new Error(`Unable to sync authorized workspaces (${response.status}).`);
    this.syncedSignature = signature;
    return workspaces;
  }

  private async childFor(workspace: AuthorizedWorkspace) {
    const existing = this.children.get(workspace.workspaceId);
    if (existing && existing.exitCode == null && !existing.killed) return existing;

    const entry = fileURLToPath(new URL("./client-entry-v2.js", import.meta.url));
    const child = spawn(process.execPath, [entry], {
      env: {
        ...process.env,
        PROJECT_ROOT: workspace.localPath,
        SERVER_URL: this.options.serverUrl,
        CODELOCAL_WORKSPACE_ID: workspace.workspaceId,
        CODELOCAL_WORKSPACE_NAME: workspace.workspaceName,
        CODELOCAL_ALLOW_SHELL: process.env.CODELOCAL_ALLOW_SHELL ?? "1",
        CODELOCAL_APPROVAL_MODE: process.env.CODELOCAL_APPROVAL_MODE ?? "prompt",
        CODELOCAL_LOG_FORMAT: process.env.CODELOCAL_LOG_FORMAT ?? "pretty",
        CODELOCAL_DAEMON_CHILD: "1",
      },
      stdio: "inherit",
      shell: false,
    });
    this.children.set(workspace.workspaceId, child);
    child.once("exit", () => {
      if (this.children.get(workspace.workspaceId) === child) this.children.delete(workspace.workspaceId);
    });
    await this.registry.markActivated(workspace.workspaceId);
    return child;
  }

  async activate(workspaceId: string) {
    const workspace = await this.registry.get(workspaceId);
    if (!workspace) throw new Error(`Workspace is not authorized on this machine: ${workspaceId}`);
    await this.childFor(workspace);
    return workspace;
  }

  private async revokeWorkspace(workspaceId: string) {
    const workspace = await this.registry.get(workspaceId);
    const removed = await this.registry.revoke(workspaceId);
    if (!removed) return false;
    const child = this.children.get(workspaceId);
    if (child && child.exitCode == null && !child.killed) child.kill("SIGTERM");
    this.children.delete(workspaceId);
    await this.syncRegistry(true);
    terminalStatus("warn", "Workspace", `${workspace?.workspaceName ?? workspaceId} removed`);
    return true;
  }

  private async pollOnce() {
    const response = await fetch(`${this.options.baseUrl}/api/client/runtime/poll`, {
      method: "POST",
      headers: deviceHeaders(this.options.credential),
      body: JSON.stringify({ workspaceIds: this.workspaces.map((workspace) => workspace.workspaceId) }),
      signal: AbortSignal.timeout(10_000),
    });
    if (response.status === 401 || response.status === 403) throw new Error("CodeLocal runtime device authorization was revoked.");
    if (!response.ok) throw new Error(`Runtime poll failed (${response.status}).`);
    return await response.json() as ActivationResponse;
  }

  async run() {
    const workspaces = await this.syncRegistry(true);
    terminalHeader();
    terminalStatus("success", "Runtime", "Online");
    terminalStatus("success", "Cloud", "Connected");
    terminalStatus("info", "Workspaces", `${workspaces.length} authorized`);
    if (!workspaces.length) terminalStatus("warn", "Workspace", "None granted · run codelocal grant /path/to/project");
    terminalStatus("muted", "Status", "Waiting for ChatGPT…");

    while (!this.stopped) {
      try {
        await this.syncRegistry();
        const message = await this.pollOnce();
        if (message.revocation?.workspaceId) {
          await this.revokeWorkspace(message.revocation.workspaceId);
          continue;
        }
        if (message.activation?.workspaceId) {
          const workspace = await this.activate(message.activation.workspaceId);
          terminalStatus("accent", "ChatGPT", `Activated ${workspace.workspaceName}`);
        }
      } catch (error) {
        if (this.stopped) break;
        terminalStatus("error", "Runtime", error instanceof Error ? error.message : String(error));
        await sleep(Math.max(1500, this.options.pollMs ?? 2500));
      }
      if (!this.stopped) await sleep(this.options.pollMs ?? 2500);
    }
  }

  status() {
    return {
      running: !this.stopped,
      pid: process.pid,
      authorizedWorkspaces: this.workspaces.map(({ workspaceId, workspaceName }) => ({ workspaceId, workspaceName })),
      activeWorkspaces: [...this.children.entries()]
        .filter(([, child]) => child.exitCode == null && !child.killed)
        .map(([workspaceId]) => workspaceId),
    };
  }

  async stop() {
    this.stopped = true;
    for (const child of this.children.values()) {
      if (child.exitCode == null && !child.killed) child.kill("SIGTERM");
    }
    this.children.clear();
  }
}
