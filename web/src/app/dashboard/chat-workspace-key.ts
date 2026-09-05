export type WorkspaceIdentity = {
  deviceId: string;
  workspaceId: string;
};

export type ThreadIdentity = {
  workspaceKey?: string;
};

export function workspaceIdentityKey(workspace: WorkspaceIdentity) {
  return `${workspace.deviceId}::${workspace.workspaceId}`;
}

export function threadBelongsToWorkspace(thread: ThreadIdentity, workspace: WorkspaceIdentity) {
  return thread.workspaceKey === workspaceIdentityKey(workspace);
}

export function projectTreeOpen(key: string, query: string, expanded: ReadonlySet<string>) {
  return query.trim().length > 0 || expanded.has(key);
}

export function toggleExpandedProject(current: ReadonlySet<string>, key: string) {
  const next = new Set(current);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  return next;
}
