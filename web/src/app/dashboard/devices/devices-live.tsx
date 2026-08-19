"use client";

import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource } from "@/lib/contracts/resources";
import { DashboardListControls, dashboardPageSize } from "../dashboard-list-controls";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
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
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);

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
  const needle = query.trim().toLowerCase();
  const filtered = needle
    ? resource.items.filter((device) => `${device.deviceName} ${device.deviceId}`.toLowerCase().includes(needle))
    : resource.items;
  const totalPages = Math.max(1, Math.ceil(filtered.length / dashboardPageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = filtered.slice((currentPage - 1) * dashboardPageSize, currentPage * dashboardPageSize);

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

      <DashboardListControls
        query={query}
        onQueryChange={setQuery}
        page={currentPage}
        totalPages={totalPages}
        totalResults={filtered.length}
        onPageChange={setPage}
        placeholder="Search device name or ID"
      />

      <div className={styles.resourceList}>
        {visible.length === 0 ? (
          <p className={styles.emptyCopy}>{query ? "No devices match your search." : "No paired device is available yet."}</p>
        ) : visible.map((device) => (
          <article className={styles.resourceRow} key={device.deviceId}>
            <span className={styles.stateDot} data-state={device.status} />
            <div className={styles.resourceIdentity}>
              <strong>{device.deviceName}</strong>
              <span>Paired {formatDashboardTime(device.createdAt)} · last seen {formatDashboardTime(device.lastSeenAt)}</span>
            </div>
            <div className={styles.resourceActions}>
              <span className={styles.resourceStatus}>{statusLabel(device.status)}</span>
              <ResourceMutationButton
                endpoint={`/api/v1/devices/${encodeURIComponent(device.deviceId)}/revoke`}
                csrf={csrf}
                label="Revoke"
                confirmMessage={`Revoke ${device.deviceName}? This disconnects its CodeLocal credential but does not delete local files.`}
                disabled={device.status === "revoked"}
                onSuccess={retry}
              />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
