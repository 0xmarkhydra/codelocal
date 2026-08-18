export type DashboardWorkspaceStatus = "active" | "sleeping" | "offline";

export type DashboardUsageWindow = {
  calls: number;
  inputTokensEstimated: number;
  outputTokensEstimated: number;
  totalTokensEstimated: number;
};

export type DashboardOverview = {
  user: {
    email: string;
    isAdmin: boolean;
  };
  devices: {
    paired: number;
    online: number;
  };
  workspaces: {
    total: number;
    active: number;
    sleeping: number;
    offline: number;
    recent: Array<{
      deviceId: string;
      deviceName: string;
      workspaceId: string;
      workspaceName: string;
      status: DashboardWorkspaceStatus;
      runtimeOnline: boolean;
      lastSeenAt: number;
    }>;
  };
  usage: {
    available: boolean;
    estimated: true;
    scope: string;
    last24h: DashboardUsageWindow;
    last30d: DashboardUsageWindow;
    allTime: DashboardUsageWindow;
  };
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isUsageWindow(value: unknown): value is DashboardUsageWindow {
  if (!isRecord(value)) return false;
  return (
    isFiniteNumber(value.calls) &&
    isFiniteNumber(value.inputTokensEstimated) &&
    isFiniteNumber(value.outputTokensEstimated) &&
    isFiniteNumber(value.totalTokensEstimated)
  );
}

function isWorkspaceStatus(value: unknown): value is DashboardWorkspaceStatus {
  return value === "active" || value === "sleeping" || value === "offline";
}

function isWorkspace(value: unknown): value is DashboardOverview["workspaces"]["recent"][number] {
  if (!isRecord(value)) return false;
  return (
    typeof value.deviceId === "string" &&
    typeof value.deviceName === "string" &&
    typeof value.workspaceId === "string" &&
    typeof value.workspaceName === "string" &&
    isWorkspaceStatus(value.status) &&
    typeof value.runtimeOnline === "boolean" &&
    isFiniteNumber(value.lastSeenAt)
  );
}

export function isDashboardOverview(value: unknown): value is DashboardOverview {
  if (!isRecord(value)) return false;
  const { user, devices, workspaces, usage } = value;
  if (!isRecord(user) || !isRecord(devices) || !isRecord(workspaces) || !isRecord(usage)) return false;
  if (
    typeof user.email !== "string" ||
    typeof user.isAdmin !== "boolean" ||
    !isFiniteNumber(devices.paired) ||
    !isFiniteNumber(devices.online) ||
    !isFiniteNumber(workspaces.total) ||
    !isFiniteNumber(workspaces.active) ||
    !isFiniteNumber(workspaces.sleeping) ||
    !isFiniteNumber(workspaces.offline) ||
    !Array.isArray(workspaces.recent) ||
    !workspaces.recent.every(isWorkspace)
  ) {
    return false;
  }
  return (
    typeof usage.available === "boolean" &&
    usage.estimated === true &&
    typeof usage.scope === "string" &&
    isUsageWindow(usage.last24h) &&
    isUsageWindow(usage.last30d) &&
    isUsageWindow(usage.allTime)
  );
}

// Canonical backend endpoint: GET /api/v1/dashboard/overview
// Keep this contract intentionally smaller than internal Go Store/Workspace
// structs. Browser/mobile/desktop clients must not depend on credentials,
// project roots, capabilities, gateway internals or persistence details.
