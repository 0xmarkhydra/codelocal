"use client";

import Link from "next/link";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource, isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function countDeviceStates(items: Array<{ status: string }>) {
  return items.reduce(
    (counts, item) => {
      if (item.status === "online") counts.online++;
      if (item.status === "revoked") counts.revoked++;
      return counts;
    },
    { online: 0, revoked: 0 },
  );
}

function countWorkspaceStates(items: Array<{ status: string }>) {
  return items.reduce(
    (counts, item) => {
      if (item.status === "active") counts.active++;
      if (item.status === "sleeping") counts.sleeping++;
      if (item.status === "offline") counts.offline++;
      return counts;
    },
    { active: 0, sleeping: 0, offline: 0 },
  );
}

export function LiveSecurity() {
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const devices = useDashboardResource("/api/v1/devices", isDevicesResource);
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);

  if (account.state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Security context"
        {...(account.state.kind === "error"
          ? { kind: "error" as const, message: account.state.message, onRetry: account.retry }
          : { kind: account.state.kind })}
      />
    );
  }

  const deviceItems = devices.state.kind === "ready" ? devices.state.value.items : [];
  const workspaceItems = workspaces.state.kind === "ready" ? workspaces.state.value.items : [];
  const deviceStates = countDeviceStates(deviceItems);
  const workspaceStates = countWorkspaceStates(workspaceItems);
  const deviceAvailable = devices.state.kind === "ready";
  const workspaceAvailable = workspaces.state.kind === "ready";

  return (
    <>
      <section className={account.state.value.requiresReauthentication ? styles.securityRisk : styles.securityHealthy}>
        <div>
          <span className={styles.eyebrow}>Browser session</span>
          <h2>{account.state.value.requiresReauthentication ? "Sensitive actions require fresh authentication." : "Current session has no pending reauthentication requirement."}</h2>
          <p>
            Go evaluates the security-device cookie, browser User-Agent and trusted network signal. High-risk changes invalidate the session; softer changes require reauthentication before sensitive mutations.
          </p>
        </div>
        <span className={styles.liveBadge}>{account.state.value.requiresReauthentication ? "REAUTH REQUIRED" : "GO VERIFIED"}</span>
      </section>

      <section className={styles.securityGrid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>Trusted machines</span><h3>Device credentials</h3></div>
            <span className={styles.badge}>{deviceAvailable ? `${deviceItems.length} records` : "Unavailable"}</span>
          </div>
          <dl className={styles.securityMetrics}>
            <div><dt>Online</dt><dd>{deviceAvailable ? deviceStates.online : "—"}</dd></div>
            <div><dt>Revoked</dt><dd>{deviceAvailable ? deviceStates.revoked : "—"}</dd></div>
            <div><dt>Other paired</dt><dd>{deviceAvailable ? Math.max(0, deviceItems.length - deviceStates.online - deviceStates.revoked) : "—"}</dd></div>
          </dl>
          <p>Revoke a machine you no longer trust. Revocation disconnects its cloud credential without deleting local project files.</p>
          <Link className={styles.liveAction} href="/dashboard/devices">Review devices</Link>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>Local authorization</span><h3>Workspace trust</h3></div>
            <span className={styles.badge}>{workspaceAvailable ? `${workspaceItems.length} authorized` : "Unavailable"}</span>
          </div>
          <dl className={styles.securityMetrics}>
            <div><dt>Active</dt><dd>{workspaceAvailable ? workspaceStates.active : "—"}</dd></div>
            <div><dt>Sleeping</dt><dd>{workspaceAvailable ? workspaceStates.sleeping : "—"}</dd></div>
            <div><dt>Offline</dt><dd>{workspaceAvailable ? workspaceStates.offline : "—"}</dd></div>
          </dl>
          <p>Workspace access is explicit and folder-scoped. Removing authorization does not delete or modify the project itself.</p>
          <Link className={styles.liveAction} href="/dashboard/workspaces">Review workspaces</Link>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>Password</span><h3>Account security</h3></div>
          </div>
          <p>Changing the password verifies the current password, requires CSRF + fresh security context, rotates security version and revokes other signed-in browser sessions.</p>
          <Link className={styles.liveAction} href="/dashboard/account-preview">Manage password</Link>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>Privacy boundary</span><h3>No cloud audit feed</h3></div>
          </div>
          <p>
            CodeLocal intentionally does not expose cloud security/audit history through this dashboard. This surface therefore does not fabricate login events, IP history or fingerprint telemetry.
          </p>
          <span className={styles.securityStatement}>Audit remains server-side operational evidence.</span>
        </article>
      </section>
    </>
  );
}
