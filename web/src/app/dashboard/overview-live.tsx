"use client";

import { DashboardOverview, isDashboardOverview } from "@/lib/contracts/dashboard";
import { DashboardResourceFeedback } from "./dashboard-resource-feedback";
import styles from "./dashboard.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

const compactNumber = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

function workspaceStateLabel(status: DashboardOverview["workspaces"]["recent"][number]["status"]) {
  switch (status) {
    case "active":
      return "Active";
    case "sleeping":
      return "Sleeping";
    default:
      return "Offline";
  }
}

export function LiveOverview() {
  const { state, retry } = useDashboardResource("/api/v1/dashboard/overview", isDashboardOverview);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Authenticated overview"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const { overview } = { overview: state.value };
  return (
    <section className={styles.livePanel} aria-live="polite">
      <div className={styles.liveHead}>
        <div>
          <span className={styles.eyebrow}>Authenticated overview</span>
          <h2>Real state from the Go authority.</h2>
          <p>Signed in as {overview.user.email}. Values below come from the versioned dashboard API.</p>
        </div>
        <span className={styles.liveBadge}>LIVE BACKEND DATA</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}>
          <span>Devices online</span>
          <strong>{overview.devices.online}<small> / {overview.devices.paired}</small></strong>
          <p>Online / paired, excluding revoked devices.</p>
        </article>
        <article className={styles.metricCard}>
          <span>Workspaces</span>
          <strong>{overview.workspaces.total}</strong>
          <p>{overview.workspaces.active} active · {overview.workspaces.sleeping} sleeping · {overview.workspaces.offline} offline</p>
        </article>
        <article className={styles.metricCard}>
          <span>MCP tokens · 24h</span>
          <strong>{overview.usage.available ? compactNumber.format(overview.usage.last24h.totalTokensEstimated) : "—"}</strong>
          <p>{overview.usage.available ? `${compactNumber.format(overview.usage.last24h.calls)} tool calls · estimate` : "Usage service unavailable"}</p>
        </article>
      </div>

      <div className={styles.recentBlock}>
        <div className={styles.recentHead}>
          <strong>Recent workspaces</strong>
          <span>{overview.usage.scope}</span>
        </div>
        {overview.workspaces.recent.length === 0 ? (
          <p className={styles.emptyCopy}>No workspace state is currently available.</p>
        ) : (
          <div className={styles.recentList}>
            {overview.workspaces.recent.map((workspace) => (
              <div className={styles.recentRow} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
                <span className={styles.stateDot} data-state={workspace.status} />
                <div>
                  <strong>{workspace.workspaceName}</strong>
                  <small>{workspace.deviceName}</small>
                </div>
                <span>{workspaceStateLabel(workspace.status)}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
