export type RuntimeScope = "global" | "device" | "workspace";

export type RuntimeSystemProject = {
  id: string;
  name: string;
  path?: string;
  source?: string;
  version?: string;
  systemApp?: boolean;
  managed: boolean;
  hidden: boolean;
  enabled: boolean;
};

export type RuntimeLayer = {
  scope: RuntimeScope;
  values?: Record<string, string>;
  secrets?: Record<string, { configured: boolean }>;
  systemProjects?: RuntimeSystemProject[];
  updatedAt?: number;
};

export type RuntimeSettingsResource = {
  scope: RuntimeScope;
  deviceId: string;
  workspaceId: string;
  layer: RuntimeLayer;
  effective: {
    values?: Record<string, string>;
    secrets?: Record<string, { configured: boolean }>;
    systemProjects?: RuntimeSystemProject[];
    version: number;
    updatedAt: number;
  };
};

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringMap(value: unknown) {
  return value === undefined || (record(value) && Object.values(value).every((item) => typeof item === "string"));
}

function secretMap(value: unknown) {
  return value === undefined || (record(value) && Object.values(value).every((item) => record(item) && typeof item.configured === "boolean"));
}

function projects(value: unknown) {
  return value === undefined || (Array.isArray(value) && value.every((item) => record(item) && typeof item.id === "string" && typeof item.name === "string" && (item.systemApp === undefined || typeof item.systemApp === "boolean") && typeof item.managed === "boolean" && typeof item.hidden === "boolean" && typeof item.enabled === "boolean"));
}

export function isRuntimeSettingsResource(value: unknown): value is RuntimeSettingsResource {
  if (!record(value) || !record(value.layer) || !record(value.effective)) return false;
  if (value.scope !== "global" && value.scope !== "device" && value.scope !== "workspace") return false;
  return typeof value.deviceId === "string" && typeof value.workspaceId === "string" &&
    value.layer.scope === value.scope && stringMap(value.layer.values) && secretMap(value.layer.secrets) && projects(value.layer.systemProjects) &&
    stringMap(value.effective.values) && secretMap(value.effective.secrets) && projects(value.effective.systemProjects) &&
    typeof value.effective.version === "number" && typeof value.effective.updatedAt === "number";
}
