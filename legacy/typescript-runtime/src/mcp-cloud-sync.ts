// Legacy compatibility shim. MCP configuration is local-only and must never be
// sourced from CodeLocal Cloud. Keep the old export names temporarily so stale
// compiled consumers fail closed during upgrades.
export type CloudMcpSyncEntry = {
  id: string;
  name: string;
  enabled: boolean;
  scope: "global" | "workspace";
  workspaceId?: string;
  transport: "stdio" | "http";
  config: Record<string, unknown>;
  requiredSecrets: string[];
  updatedAt: number;
};

export async function syncCloudMcpInstallations(_workspaceRoot: string, _entries: CloudMcpSyncEntry[]) {
  return { applied: 0, skipped: true, reason: "MCP configuration is local-only" };
}
