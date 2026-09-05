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
