"use client";

import Link from "next/link";
import { DashboardOverview, isDashboardOverview } from "@/lib/contracts/dashboard";
import controls from "./dashboard-controls.module.css";
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

function onboardingCopy(overview: DashboardOverview) {
  if (overview.devices.paired === 0) {
    return {
      eyebrow: "Setup required",
      title: "Pair your first machine to get started.",
      copy: "Install CodeLocal, pair this computer, then authorize only the project folders you want compatible AI clients to use.",
    };
  }
  if (overview.devices.online === 0) {
    return {
      eyebrow: "Runtime offline",
      title: "Your projects are ready when your runtime is.",
      copy: `${overview.devices.paired} paired machine${overview.devices.paired === 1 ? "" : "s"} · ${overview.workspaces.total} authorized workspace${overview.workspaces.total === 1 ? "" : "s"}. Start CodeLocal on a paired machine to make them available to connected MCP clients.`,
    };
  }
  return {
    eyebrow: "Runtime connected",
    title: "Your CodeLocal environment is ready.",
    copy: `${overview.devices.online} of ${overview.devices.paired} paired machine runtime${overview.devices.paired === 1 ? "" : "s"} online · ${overview.workspaces.active} active workspace${overview.workspaces.active === 1 ? "" : "s"} · ${overview.workspaces.sleeping} sleeping.`,
  };
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

  const overview = state.value;
  const onboarding = onboardingCopy(overview);
  return (
    <>
      <section className={controls.overviewState}>
        <div>
          <span className={styles.eyebrow}>{onboarding.eyebrow}</span>
          <h2>{onboarding.title}</h2>
          <p>{onboarding.copy}</p>
        </div>
        <div className={controls.overviewStateActions}>
          <a href="/dashboard/connect">Connect an AI client</a>
          <Link href="/dashboard/workspaces">View workspaces</Link>
          <Link href="/dashboard/code-graph">Explore Code Graph</Link>
        </div>
      </section>

      <section className={styles.livePanel} aria-live="polite">
        <div className={styles.liveHead}>
          <div>
            <span className={styles.eyebrow}>Current state</span>
            <h2>Your workspace at a glance.</h2>
            <p>Signed in as {overview.user.email}. Device, workspace and usage totals reflect your current CodeLocal account.</p>
          </div>
          <span className={styles.liveBadge}>Live</span>
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
    </>
  );
}
