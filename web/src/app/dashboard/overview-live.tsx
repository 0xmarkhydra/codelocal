"use client";

import Link from "next/link";
import { DashboardOverview, isDashboardOverview } from "@/lib/contracts/dashboard";
import { DashboardResourceFeedback } from "./dashboard-resource-feedback";
import styles from "./dashboard.module.css";
import overviewStyles from "./overview.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

const compactNumber = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

const nodePositions = [
  [90, 88],
  [198, 52],
  [314, 92],
  [428, 54],
  [514, 132],
  [418, 202],
  [284, 226],
  [142, 198],
] as const;

function workspaceStateLabel(status: DashboardOverview["workspaces"]["recent"][number]["status"]) {
  switch (status) {
    case "active": return "Active";
    case "sleeping": return "Sleeping";
    default: return "Offline";
  }
}

function onboardingCopy(overview: DashboardOverview) {
  if (overview.devices.paired === 0) {
    return {
      eyebrow: "Setup required",
      title: "Pair your first machine.",
      copy: "Install CodeLocal, pair this computer, then authorize the projects you want AI clients to use.",
    };
  }
  if (overview.devices.online === 0) {
    return {
      eyebrow: "Runtime offline",
      title: "Your project brain is waiting for a runtime.",
      copy: `${overview.devices.paired} paired machine${overview.devices.paired === 1 ? "" : "s"} · ${overview.workspaces.total} authorized workspace${overview.workspaces.total === 1 ? "" : "s"}.`,
    };
  }
  return {
    eyebrow: "Runtime connected",
    title: "Your project brain is live.",
    copy: `${overview.devices.online}/${overview.devices.paired} machine runtime online · ${overview.workspaces.active} active workspace${overview.workspaces.active === 1 ? "" : "s"}.`,
  };
}

function usageHeight(value: number, max: number) {
  if (max <= 0 || value <= 0) return "7%";
  return `${Math.max(12, Math.round((value / max) * 100))}%`;
}

function OverviewBrain({ overview }: { overview: DashboardOverview }) {
  const visible = overview.workspaces.recent.slice(0, nodePositions.length);
  const labels = visible.slice(0, 4);

  return (
    <div className={overviewStyles.brainPane} aria-label="Live project brain summary">
      <svg
        className={overviewStyles.brainSvg}
        viewBox="0 0 600 280"
        role="img"
        aria-label={`${overview.workspaces.total} authorized workspaces in the current account; ${visible.length} recent workspaces visualized`}
      >
        <defs>
          <linearGradient id="overview-edge" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#42dcff" />
            <stop offset="0.5" stopColor="#7f70ff" />
            <stop offset="1" stopColor="#ff62d9" />
          </linearGradient>
        </defs>
        <g className={overviewStyles.brainEdges}>
          {visible.map((_, index) => {
            const [x, y] = nodePositions[index];
            const next = nodePositions[(index + 2) % nodePositions.length];
            return (
              <path
                key={`${x}:${y}`}
                d={`M300 140 C${(300 + x) / 2} ${(140 + y) / 2 - 20} ${(300 + next[0]) / 2} ${(140 + next[1]) / 2 + 18} ${x} ${y}`}
              />
            );
          })}
        </g>
        <g className={overviewStyles.brainNodes}>
          <circle cx="300" cy="140" r="13" data-state="core" />
          {visible.map((workspace, index) => {
            const [x, y] = nodePositions[index];
            return (
              <circle
                key={`${workspace.deviceId}:${workspace.workspaceId}`}
                cx={x}
                cy={y}
                r={workspace.status === "active" ? 7 : 5.5}
                data-state={workspace.status}
              />
            );
          })}
        </g>
      </svg>
      {labels.map((workspace, index) => (
        <span
          className={overviewStyles.brainLabel}
          data-index={index}
          key={`${workspace.deviceId}:${workspace.workspaceId}:label`}
          title={workspace.workspaceName}
        >
          {workspace.workspaceName}
        </span>
      ))}
      <span className={overviewStyles.brainMeta}>Live account state · recent workspace map</span>
    </div>
  );
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
  const usage = [
    overview.usage.last24h.totalTokensEstimated,
    overview.usage.last30d.totalTokensEstimated,
    overview.usage.allTime.totalTokensEstimated,
  ];
  const usageMax = Math.max(...usage, 1);

  return (
    <div className={overviewStyles.overviewShell}>
      <section className={overviewStyles.commandDeck}>
        <div className={overviewStyles.statusPane}>
          <span className={styles.eyebrow}>{onboarding.eyebrow}</span>
          <h2>{onboarding.title}</h2>
          <p>{onboarding.copy}</p>
          <div className={overviewStyles.statusActions}>
            <Link href="/dashboard/connect">Connect AI</Link>
            <Link href="/dashboard/workspaces">Workspaces</Link>
            <Link href="/dashboard/code-graph">Code Graph</Link>
          </div>
          <div className={overviewStyles.metricStrip}>
            <article>
              <span>Machines</span>
              <strong>{overview.devices.online}/{overview.devices.paired}</strong>
              <small>online / paired</small>
            </article>
            <article>
              <span>Workspaces</span>
              <strong>{overview.workspaces.total}</strong>
              <small>{overview.workspaces.active} active · {overview.workspaces.sleeping} sleeping</small>
            </article>
            <article>
              <span>MCP · 24h</span>
              <strong>{overview.usage.available ? compactNumber.format(overview.usage.last24h.totalTokensEstimated) : "—"}</strong>
              <small>{overview.usage.available ? `${compactNumber.format(overview.usage.last24h.calls)} calls` : "unavailable"}</small>
            </article>
          </div>
        </div>
        <OverviewBrain overview={overview} />
      </section>

      <div className={overviewStyles.lowerGrid}>
        <section className={overviewStyles.compactPanel}>
          <div className={overviewStyles.panelHead}>
            <strong>MCP activity</strong>
            <span>Estimated payload tokens · cumulative windows</span>
          </div>
          <div className={overviewStyles.usageBars} aria-label="Estimated MCP payload token totals by cumulative window">
            {usage.map((value, index) => (
              <div
                className={overviewStyles.usageBar}
                style={{ height: usageHeight(value, usageMax) }}
                key={index}
              >
                <span>{overview.usage.available ? compactNumber.format(value) : "—"}</span>
              </div>
            ))}
          </div>
          <div className={overviewStyles.usageLabels}><span>24h</span><span>30d</span><span>All time</span></div>
        </section>

        <section className={overviewStyles.compactPanel}>
          <div className={overviewStyles.panelHead}>
            <strong>Recent workspaces</strong>
            <span>{overview.usage.scope}</span>
          </div>
          {overview.workspaces.recent.length === 0 ? (
            <p className={overviewStyles.empty}>No workspace state is currently available.</p>
          ) : (
            <div className={overviewStyles.workspaceList}>
              {overview.workspaces.recent.slice(0, 5).map((workspace) => (
                <div className={overviewStyles.workspaceRow} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
                  <span className={overviewStyles.workspaceDot} data-state={workspace.status} />
                  <div>
                    <strong>{workspace.workspaceName}</strong>
                    <small>{workspace.deviceName}</small>
                  </div>
                  <span>{workspaceStateLabel(workspace.status)}</span>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
