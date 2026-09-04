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

const windows: Array<{ key: "last1h" | "last24h" | "last30d" | "allTime"; label: string }> = [
  { key: "last1h", label: "Last hour" },
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
        <div><dt>Input</dt><dd>{exactNumber.format(value.inputTokensEstimated)}</dd></div>
        <div><dt>Output</dt><dd>{exactNumber.format(value.outputTokensEstimated)}</dd></div>
        <div><dt>Total</dt><dd>{exactNumber.format(value.totalTokensEstimated)}</dd></div>
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
        <div><span className={styles.eyebrow}>MCP</span></div>
        <span className={styles.liveBadge}>Live</span>
      </div>

      <div className={`${styles.metricGrid} ${styles.usageMetricGrid}`}>
        <article className={styles.metricCard}><span>1h</span><strong>{compactNumber.format(resource.last1h.totalTokensEstimated)}</strong></article>
        <article className={styles.metricCard}><span>24h</span><strong>{compactNumber.format(resource.last24h.totalTokensEstimated)}</strong></article>
        <article className={styles.metricCard}><span>30d</span><strong>{compactNumber.format(resource.last30d.totalTokensEstimated)}</strong></article>
        <article className={styles.metricCard}><span>All time</span><strong>{compactNumber.format(resource.allTime.totalTokensEstimated)}</strong></article>
      </div>

      <div className={styles.usageList}>
        {windows.map((window) => (
          <UsageBreakdown label={window.label} value={resource[window.key]} key={window.key} />
        ))}
      </div>
    </section>
  );
}
