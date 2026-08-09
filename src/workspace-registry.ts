import path from "node:path";
import os from "node:os";
import { createHash } from "node:crypto";
import { promises as fs } from "node:fs";
import { DEFAULT_STATE_DIR, readJsonFile, writeJsonAtomic } from "./state.js";

export type AuthorizedWorkspace = {
  workspaceId: string;
  workspaceName: string;
  localPath: string;
  grantedAt: number;
  lastActivatedAt?: number;
};

type WorkspaceRegistryFile = {
  version: 1;
  workspaces: AuthorizedWorkspace[];
};

export function workspaceIdForPath(project: string) {
  const normalized = path.resolve(project);
  const slug = path.basename(normalized).replace(/[^A-Za-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 48) || "workspace";
  const digest = createHash("sha256").update(normalized).digest("hex").slice(0, 10);
  return `${slug}-${digest}`;
}

function unsafeBroadGrant(project: string) {
  const root = path.parse(project).root;
  const home = path.resolve(os.homedir());
  return project === root || project === home;
}

export class WorkspaceRegistry {
  constructor(private file = path.join(DEFAULT_STATE_DIR, "workspaces.json")) {}

  private async read(): Promise<WorkspaceRegistryFile> {
    const value = await readJsonFile<WorkspaceRegistryFile>(this.file, { version: 1, workspaces: [] });
    return { version: 1, workspaces: Array.isArray(value.workspaces) ? value.workspaces : [] };
  }

  private async write(workspaces: AuthorizedWorkspace[]) {
    const unique = new Map(workspaces.map((workspace) => [workspace.workspaceId, workspace]));
    await writeJsonAtomic(this.file, { version: 1, workspaces: [...unique.values()].sort((a, b) => a.workspaceName.localeCompare(b.workspaceName)) });
  }

  async list() {
    const data = await this.read();
    const valid: AuthorizedWorkspace[] = [];
    for (const workspace of data.workspaces) {
      const exists = await fs.stat(workspace.localPath).then((stat) => stat.isDirectory()).catch(() => false);
      if (exists) valid.push(workspace);
    }
    return valid;
  }

  async grant(projectArg: string, workspaceName?: string) {
    const project = await fs.realpath(path.resolve(projectArg));
    const stat = await fs.stat(project);
    if (!stat.isDirectory()) throw new Error("Only directories can be granted as CodeLocal workspaces.");
    if (unsafeBroadGrant(project)) throw new Error("Refusing to grant the filesystem root or entire home directory. Grant a specific project folder instead.");
    if (/(^|[\\/])\.(ssh|aws|gnupg|gcloud|azure)([\\/]|$)/i.test(project)) throw new Error("Credential/config directories cannot be granted as CodeLocal workspaces.");

    const existing = await this.read();
    const workspaceId = workspaceIdForPath(project);
    const previous = existing.workspaces.find((item) => item.workspaceId === workspaceId);
    const entry: AuthorizedWorkspace = {
      workspaceId,
      workspaceName: (workspaceName?.trim() || path.basename(project)).slice(0, 120),
      localPath: project,
      grantedAt: previous?.grantedAt ?? Date.now(),
      lastActivatedAt: previous?.lastActivatedAt,
    };
    await this.write([...existing.workspaces.filter((item) => item.workspaceId !== workspaceId), entry]);
    return entry;
  }

  async revoke(identifier: string) {
    const data = await this.read();
    let resolvedPath: string | null = null;
    try { resolvedPath = await fs.realpath(path.resolve(identifier)); } catch {}
    const next = data.workspaces.filter((workspace) => workspace.workspaceId !== identifier && workspace.localPath !== resolvedPath);
    if (next.length === data.workspaces.length) return false;
    await this.write(next);
    return true;
  }

  async get(workspaceId: string) {
    return (await this.list()).find((workspace) => workspace.workspaceId === workspaceId) ?? null;
  }

  async markActivated(workspaceId: string) {
    const data = await this.read();
    const workspace = data.workspaces.find((item) => item.workspaceId === workspaceId);
    if (!workspace) return;
    workspace.lastActivatedAt = Date.now();
    await this.write(data.workspaces);
  }
}
