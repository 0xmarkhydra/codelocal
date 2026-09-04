export type UsageWindow = {
  calls: number;
  inputTokensEstimated: number;
  outputTokensEstimated: number;
  totalTokensEstimated: number;
};

export type UsageResource = {
  estimated: true;
  scope: string;
  last1h: UsageWindow;
  last24h: UsageWindow;
  last30d: UsageWindow;
  allTime: UsageWindow;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isUsageWindow(value: unknown): value is UsageWindow {
  if (!isRecord(value)) return false;
  return (
    isFiniteNumber(value.calls) &&
    isFiniteNumber(value.inputTokensEstimated) &&
    isFiniteNumber(value.outputTokensEstimated) &&
    isFiniteNumber(value.totalTokensEstimated)
  );
}

export function isUsageResource(value: unknown): value is UsageResource {
  if (!isRecord(value)) return false;
  return (
    value.estimated === true &&
    typeof value.scope === "string" &&
    isUsageWindow(value.last1h) &&
    isUsageWindow(value.last24h) &&
    isUsageWindow(value.last30d) &&
    isUsageWindow(value.allTime)
  );
}
