"use client";

import { useEffect, useState } from "react";
import { DashboardOverview, isDashboardOverview } from "@/lib/contracts/dashboard";
import styles from "./dashboard.module.css";

type OverviewState =
  | { kind: "loading" }
  | { kind: "ready"; overview: DashboardOverview }
  | { kind: "unauthenticated" }
  | { kind: "error"; message: string };

const compactNumber = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

function workspaceStateLabel(status: "active" | "sleeping" | "offline") {
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
  const [state, setState] = useState<OverviewState>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        const response = await fetch("/api/v1/dashboard/overview", {
          method: "GET",
          credentials: "same-origin",
          cache: "no-store",
          headers: { Accept: "application/json" },
          signal: controller.signal,
        });
        if (response.status === 401) {
          setState({ kind: "unauthenticated" });
          return;
        }
        if (!response.ok) {
          setState({ kind: "error", message: `Backend returned ${response.status}.` });
          return;
        }
        const body: unknown = await response.json();
        if (!isDashboardOverview(body)) {
          setState({ kind: "error", message: "Backend response did not match the dashboard contract." });
          return;
        }
        setState({ kind: "ready", overview: body });
      } catch (error) {
        if (controller.signal.aborted) return;
        setState({
          kind: "error",
          message: error instanceof Error ? error.message : "Unable to reach the CodeLocal backend.",
        });
      }
    }

    void load();
    return () => controller.abort();
  }, [attempt]);

  if (state.kind === "loading") {
    return (
      <section className={styles.livePanel} aria-live="polite">
        <span className={styles.eyebrow}>Authenticated overview</span>
        <h2>Checking the Go backend…</h2>
        <p>No local placeholder values are shown while real state is loading.</p>
      </section>
    );
  }

  if (state.kind === "unauthenticated") {
    return (
      <section className={styles.livePanel} aria-live="polite">
        <span className={styles.eyebrow}>Authenticated overview</span>
        <h2>Sign in to load real CodeLocal state.</h2>
        <p>The browser session remains owned and verified by the Go backend.</p>
        <a className={styles.liveAction} href="/login?next=%2Fdashboard">Sign in through CodeLocal</a>
      </section>
    );
  }

  if (state.kind === "error") {
    return (
      <section className={styles.livePanel} aria-live="polite">
        <span className={styles.eyebrow}>Authenticated overview</span>
        <h2>Live backend state is unavailable.</h2>
        <p>{state.message} The UI will not substitute mocked activity.</p>
        <button
          className={styles.liveAction}
          type="button"
          onClick={() => {
            setState({ kind: "loading" });
            setAttempt((value) => value + 1);
          }}
        >
          Retry
        </button>
      </section>
    );
  }

  const { overview } = state;
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
