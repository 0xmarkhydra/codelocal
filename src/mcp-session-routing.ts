export type WorkspaceRoutingState = {
  selectedWorkspaceKey: string | null;
};

export function createWorkspaceRoutingState(): WorkspaceRoutingState {
  return { selectedWorkspaceKey: null };
}

export function selectWorkspaceForSession(state: WorkspaceRoutingState, workspaceKey: string) {
  const normalized = workspaceKey.trim();
  if (!normalized) throw new Error("Workspace key is required.");
  state.selectedWorkspaceKey = normalized;
  return normalized;
}

export function workspaceKeyForSession(state: WorkspaceRoutingState, explicitWorkspaceKey?: string | null) {
  const explicit = explicitWorkspaceKey?.trim();
  return explicit || state.selectedWorkspaceKey;
}
