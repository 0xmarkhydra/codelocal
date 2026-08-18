"use client";

import { isDevicesResource } from "@/lib/contracts/resources";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function statusLabel(status: "online" | "offline" | "revoked") {
  switch (status) {
    case "online":
      return "Online";
    case "revoked":
      return "Revoked";
    default:
      return "Offline";
  }
}

export function LiveDevices() {
  const { state, retry } = useDashboardResource("/api/v1/devices", isDevicesResource);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Paired devices"
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
          <span className={styles.eyebrow}>Paired devices</span>
          <h2>Machine runtimes connected to your account.</h2>
          <p>The API exposes display identity and reachability only; credential identifiers, public keys and secret hashes stay server-side.</p>
        </div>
        <span className={styles.liveBadge}>LIVE BACKEND DATA</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}><span>Paired</span><strong>{resource.summary.paired}</strong><p>Non-revoked machine credentials.</p></article>
        <article className={styles.metricCard}><span>Online</span><strong>{resource.summary.online}</strong><p>Runtime currently reachable.</p></article>
        <article className={styles.metricCard}><span>Revoked</span><strong>{resource.summary.revoked}</strong><p>Credentials no longer allowed to reconnect.</p></article>
      </div>

      <div className={styles.resourceList}>
        {resource.items.length === 0 ? (
          <p className={styles.emptyCopy}>No paired device is available yet.</p>
        ) : resource.items.map((device) => (
          <article className={styles.resourceRow} key={device.deviceId}>
            <span className={styles.stateDot} data-state={device.status} />
            <div className={styles.resourceIdentity}>
              <strong>{device.deviceName}</strong>
              <span>Paired {formatDashboardTime(device.createdAt)} · last seen {formatDashboardTime(device.lastSeenAt)}</span>
            </div>
            <span className={styles.resourceStatus}>{statusLabel(device.status)}</span>
          </article>
        ))}
      </div>
    </section>
  );
}
