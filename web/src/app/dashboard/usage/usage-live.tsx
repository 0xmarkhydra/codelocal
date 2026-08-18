"use client";

import { isUsageResource, type UsageWindow } from "@/lib/contracts/usage";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

const compactNumber = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

const exactNumber = new Intl.NumberFormat("en-US");

const windows: Array<{ key: "last24h" | "last30d" | "allTime"; label: string }> = [
  { key: "last24h", label: "Last 24 hours" },
  { key: "last30d", label: "Last 30 days" },
  { key: "allTime", label: "All time" },
];

function UsageBreakdown({ label, value }: { label: string; value: UsageWindow }) {
  return (
    <article className={styles.usageRow}>
      <div>
        <strong>{label}</strong>
        <span>{exactNumber.format(value.calls)} tool calls</span>
      </div>
      <dl>
        <div><dt>Input est.</dt><dd>{exactNumber.format(value.inputTokensEstimated)}</dd></div>
        <div><dt>Output est.</dt><dd>{exactNumber.format(value.outputTokensEstimated)}</dd></div>
        <div><dt>Total est.</dt><dd>{exactNumber.format(value.totalTokensEstimated)}</dd></div>
      </dl>
    </article>
  );
}

export function LiveUsage() {
  const { state, retry } = useDashboardResource("/api/v1/usage", isUsageResource);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="MCP usage"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const resource = state.value;
  return (
    <section className={styles.livePanel} aria-live="polite">
      <div className={styles.liveHead}>
        <div>
          <span className={styles.eyebrow}>MCP usage</span>
          <h2>Tool-call payload estimates from the Go usage authority.</h2>
          <p>{resource.scope}. These values are not a provider invoice and no USD cost is inferred here.</p>
        </div>
        <span className={styles.liveBadge}>ESTIMATED · LIVE BACKEND DATA</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}>
          <span>Last 24 hours</span>
          <strong>{compactNumber.format(resource.last24h.totalTokensEstimated)}</strong>
          <p>{exactNumber.format(resource.last24h.calls)} tool calls · estimated MCP tokens</p>
        </article>
        <article className={styles.metricCard}>
          <span>Last 30 days</span>
          <strong>{compactNumber.format(resource.last30d.totalTokensEstimated)}</strong>
          <p>{exactNumber.format(resource.last30d.calls)} tool calls · estimated MCP tokens</p>
        </article>
        <article className={styles.metricCard}>
          <span>All time</span>
          <strong>{compactNumber.format(resource.allTime.totalTokensEstimated)}</strong>
          <p>{exactNumber.format(resource.allTime.calls)} tool calls · estimated MCP tokens</p>
        </article>
      </div>

      <div className={styles.usageList}>
        {windows.map((window) => (
          <UsageBreakdown label={window.label} value={resource[window.key]} key={window.key} />
        ))}
      </div>
    </section>
  );
}
