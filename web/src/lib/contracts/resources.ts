export type DeviceStatus = "online" | "offline" | "revoked";
export type WorkspaceStatus = "active" | "sleeping" | "offline";

export type DevicesResource = {
  summary: {
    paired: number;
    online: number;
    revoked: number;
  };
  items: Array<{
    deviceId: string;
    deviceName: string;
    status: DeviceStatus;
    createdAt: number;
    lastSeenAt: number;
  }>;
};

export type WorkspacesResource = {
  summary: {
    total: number;
    active: number;
    sleeping: number;
    offline: number;
  };
  items: Array<{
    deviceId: string;
    deviceName: string;
    workspaceId: string;
    workspaceName: string;
    systemApp: boolean;
    managed: boolean;
    status: WorkspaceStatus;
    runtimeOnline: boolean;
    lastSeenAt: number;
  }>;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isDeviceStatus(value: unknown): value is DeviceStatus {
  return value === "online" || value === "offline" || value === "revoked";
}

function isWorkspaceStatus(value: unknown): value is WorkspaceStatus {
  return value === "active" || value === "sleeping" || value === "offline";
}

export function isDevicesResource(value: unknown): value is DevicesResource {
  if (!isRecord(value) || !isRecord(value.summary) || !Array.isArray(value.items)) return false;
  if (
    !isFiniteNumber(value.summary.paired) ||
    !isFiniteNumber(value.summary.online) ||
    !isFiniteNumber(value.summary.revoked)
  ) {
    return false;
  }
  return value.items.every((item) => {
    if (!isRecord(item)) return false;
    return (
      typeof item.deviceId === "string" &&
      typeof item.deviceName === "string" &&
      isDeviceStatus(item.status) &&
      isFiniteNumber(item.createdAt) &&
      isFiniteNumber(item.lastSeenAt)
    );
  });
}

export function isWorkspacesResource(value: unknown): value is WorkspacesResource {
  if (!isRecord(value) || !isRecord(value.summary) || !Array.isArray(value.items)) return false;
  if (
    !isFiniteNumber(value.summary.total) ||
    !isFiniteNumber(value.summary.active) ||
    !isFiniteNumber(value.summary.sleeping) ||
    !isFiniteNumber(value.summary.offline)
  ) {
    return false;
  }
  return value.items.every((item) => {
    if (!isRecord(item)) return false;
    return (
      typeof item.deviceId === "string" &&
      typeof item.deviceName === "string" &&
      typeof item.workspaceId === "string" &&
      typeof item.workspaceName === "string" &&
      typeof item.systemApp === "boolean" &&
      typeof item.managed === "boolean" &&
      isWorkspaceStatus(item.status) &&
      typeof item.runtimeOnline === "boolean" &&
      isFiniteNumber(item.lastSeenAt)
    );
  });
}
