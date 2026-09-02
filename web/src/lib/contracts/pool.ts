export type AIPoolResource = {
  configured: boolean;
  routingReady: boolean;
  available: boolean;
  dashboardUrl?: string;
  defaultModel?: string;
  modelCount: number;
  models: string[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isSafeHttpUrl(value: string) {
  try {
    const url = new URL(value);
    return (url.protocol === "https:" || url.protocol === "http:") && !url.username && !url.password;
  } catch {
    return false;
  }
}

function isModelId(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= 160 && /^[A-Za-z0-9._:/-]+$/.test(value);
}

export function isAIPoolResource(value: unknown): value is AIPoolResource {
  if (!isRecord(value) || !Array.isArray(value.models)) return false;
  if (
    typeof value.configured !== "boolean" ||
    typeof value.routingReady !== "boolean" ||
    typeof value.available !== "boolean" ||
    typeof value.modelCount !== "number" ||
    !Number.isInteger(value.modelCount) ||
    value.modelCount < 0 ||
    value.modelCount > 10000
  ) {
    return false;
  }
  if (value.dashboardUrl !== undefined && (typeof value.dashboardUrl !== "string" || !isSafeHttpUrl(value.dashboardUrl))) {
    return false;
  }
  if (value.defaultModel !== undefined && !isModelId(value.defaultModel)) {
    return false;
  }
  return value.models.length <= 100 && value.models.every(isModelId);
}
