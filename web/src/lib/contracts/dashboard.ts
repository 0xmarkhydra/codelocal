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

// Canonical backend endpoint: GET /api/v1/dashboard/overview
// Keep this contract intentionally smaller than internal Go Store/Workspace
// structs. Browser/mobile/desktop clients must not depend on credentials,
// project roots, capabilities, gateway internals or persistence details.
