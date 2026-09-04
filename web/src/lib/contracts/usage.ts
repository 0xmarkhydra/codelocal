export type UsageWindow = {
  calls: number;
  inputTokensEstimated: number;
  outputTokensEstimated: number;
  totalTokensEstimated: number;
};

export type ReportedUsageWindow = {
  turns: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
};

export type ReportedUsage = {
  source: "provider_reported";
  last1h: ReportedUsageWindow;
  last24h: ReportedUsageWindow;
  last30d: ReportedUsageWindow;
  allTime: ReportedUsageWindow;
};

export type EstimatedUsage = {
  source: "payload_estimated";
  last1h: UsageWindow;
  last24h: UsageWindow;
  last30d: UsageWindow;
  allTime: UsageWindow;
};

export type UsageResource = {
  estimated: true;
  scope: string;
  last1h?: UsageWindow;
  last24h: UsageWindow;
  last30d: UsageWindow;
  allTime: UsageWindow;
  webChat: ReportedUsage;
  mcp: EstimatedUsage;
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

function isReportedUsageWindow(value: unknown): value is ReportedUsageWindow {
  if (!isRecord(value)) return false;
  return (
    isFiniteNumber(value.turns) &&
    isFiniteNumber(value.inputTokens) &&
    isFiniteNumber(value.outputTokens) &&
    isFiniteNumber(value.totalTokens)
  );
}

function isReportedUsage(value: unknown): value is ReportedUsage {
  return isRecord(value) && value.source === "provider_reported" &&
    isReportedUsageWindow(value.last1h) && isReportedUsageWindow(value.last24h) &&
    isReportedUsageWindow(value.last30d) && isReportedUsageWindow(value.allTime);
}

function isEstimatedUsage(value: unknown): value is EstimatedUsage {
  return isRecord(value) && value.source === "payload_estimated" &&
    isUsageWindow(value.last1h) && isUsageWindow(value.last24h) &&
    isUsageWindow(value.last30d) && isUsageWindow(value.allTime);
}

export function isUsageResource(value: unknown): value is UsageResource {
  if (!isRecord(value)) return false;
  return (
    value.estimated === true &&
    typeof value.scope === "string" &&
    (value.last1h === undefined || isUsageWindow(value.last1h)) &&
    isUsageWindow(value.last24h) &&
    isUsageWindow(value.last30d) &&
    isUsageWindow(value.allTime) &&
    isReportedUsage(value.webChat) &&
    isEstimatedUsage(value.mcp)
  );
}
