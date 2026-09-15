"use client";

import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource } from "@/lib/contracts/resources";
import { AppIcon } from "../app-icon";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { useTranslations } from "@/lib/i18n/provider";

function statusLabel(status: "online" | "offline" | "revoked") {
  switch (status) {
    case "online": return "Online";
    case "revoked": return "Revoked";
    default: return "Offline";
  }
}

export function LiveDevices() {
  const { locale, t } = useTranslations();
  const { state, retry } = useDashboardResource("/api/v1/devices", isDevicesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [selectedID, setSelectedID] = useState<string | null>(null);

  const selected = state.kind === "ready"
    ? state.value.items.find((item) => item.deviceId === selectedID) ?? state.value.items[0] ?? null
    : null;

  if (state.kind !== "ready") {
    return <DashboardResourceFeedback label={t("Devices")} {...(state.kind === "error" ? { kind: "error" as const, message: state.message, onRetry: retry } : { kind: state.kind })} />;
  }

  const resource = state.value;

  return (
    <section className={styles.deviceShell} aria-live="polite">
      <aside className={styles.deviceList}>
        <div className={styles.deviceSummary}>
          <span>{t("{count} paired devices", { count: resource.summary.paired })}</span>
          <span data-state="online"><i />{t("{count} online", { count: resource.summary.online })}</span>
        </div>
        <div className={styles.deviceListItems}>
          {resource.items.map((device) => (
            <button
              className={selected?.deviceId === device.deviceId ? styles.deviceListItemActive : styles.deviceListItem}
              type="button"
              key={device.deviceId}
              onClick={() => setSelectedID(device.deviceId)}
            >
              <span className={styles.deviceGlyph} aria-hidden="true"><AppIcon name="device" size={20} /></span>
              <span><strong>{device.deviceName}</strong><small>{t(statusLabel(device.status))}</small></span>
              <i className={styles.deviceStateDot} data-state={device.status} />
            </button>
          ))}
        </div>
      </aside>

      <div className={styles.deviceDetail}>
        {selected ? (
          <>
            <div className={styles.deviceDetailHead}>
              <div className={styles.deviceHeroIcon} aria-hidden="true"><AppIcon name="device" size={20} /></div>
              <div>
                <h2>{selected.deviceName}</h2>
                <p><span className={styles.statusPill} data-state={selected.status}><i />{t(statusLabel(selected.status))}</span></p>
              </div>
            </div>

            <div className={styles.deviceFacts}>
              <article><span>{t("Status")}</span><strong>{t(statusLabel(selected.status))}</strong></article>
              <article><span>{t("Last active")}</span><strong>{formatDashboardTime(selected.lastSeenAt, locale)}</strong></article>
              <article><span>{t("Paired")}</span><strong>{formatDashboardTime(selected.createdAt, locale)}</strong></article>
              <article><span>{t("Device ID")}</span><strong title={selected.deviceId}>{selected.deviceId.slice(0, 12)}…</strong></article>
            </div>

            <div className={styles.deviceActionsPanel}>
              <div>
                <strong>{t("Manage device")}</strong>
                <p>{t("Revoking credentials disconnects CodeLocal.Cloud from this machine without deleting local files.")}</p>
              </div>
              <ResourceMutationButton
                endpoint={`/api/v1/devices/${encodeURIComponent(selected.deviceId)}/revoke`}
                csrf={csrf}
                label={t("Revoke")}
                confirmMessage={t("Revoke {name}? This disconnects its CodeLocal.Cloud credential but does not delete local files.", { name: selected.deviceName })}
                disabled={selected.status === "revoked"}
                onSuccess={retry}
              />
            </div>
          </>
        ) : <div className={styles.appleEmptyState}><strong>{t("No devices yet")}</strong><p>{t("Pair a machine with CodeLocal.Cloud to get started.")}</p></div>}
      </div>
    </section>
  );
}
