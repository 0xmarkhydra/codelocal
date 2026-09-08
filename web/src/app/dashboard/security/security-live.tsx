"use client";

import Link from "next/link";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource, isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { useTranslations } from "@/lib/i18n/provider";

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
  const { locale, t } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const devices = useDashboardResource("/api/v1/devices", isDevicesResource);
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);

  if (account.state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label={t("Security")}
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
        <div><h2>{t(account.state.value.requiresReauthentication ? "Sign in again" : "Session verified")}</h2></div>
        <span className={styles.liveBadge}>{t(account.state.value.requiresReauthentication ? "Reauthentication required" : "Verified")}</span>
      </section>

      <section className={styles.securityGrid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>{t("Trusted machines")}</span><h3>{t("Device credentials")}</h3></div>
            <span className={styles.badge}>{deviceAvailable ? t("{count} records", { count: deviceItems.length }) : t("Unavailable")}</span>
          </div>
          <dl className={styles.securityMetrics}>
            <div><dt>{t("Online")}</dt><dd>{deviceAvailable ? number.format(deviceStates.online) : "—"}</dd></div>
            <div><dt>{t("Revoked")}</dt><dd>{deviceAvailable ? number.format(deviceStates.revoked) : "—"}</dd></div>
            <div><dt>{t("Other paired")}</dt><dd>{deviceAvailable ? number.format(Math.max(0, deviceItems.length - deviceStates.online - deviceStates.revoked)) : "—"}</dd></div>
          </dl>
          <Link className={styles.liveAction} href="/dashboard/devices">{t("Devices")}</Link>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>{t("Local authorization")}</span><h3>{t("Workspace trust")}</h3></div>
            <span className={styles.badge}>{workspaceAvailable ? t("{count} authorized", { count: workspaceItems.length }) : t("Unavailable")}</span>
          </div>
          <dl className={styles.securityMetrics}>
            <div><dt>{t("Active")}</dt><dd>{workspaceAvailable ? number.format(workspaceStates.active) : "—"}</dd></div>
            <div><dt>{t("Sleeping")}</dt><dd>{workspaceAvailable ? number.format(workspaceStates.sleeping) : "—"}</dd></div>
            <div><dt>{t("Offline")}</dt><dd>{workspaceAvailable ? number.format(workspaceStates.offline) : "—"}</dd></div>
          </dl>
          <Link className={styles.liveAction} href="/dashboard/workspaces">{t("Workspaces")}</Link>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div><span className={styles.eyebrow}>{t("Password")}</span><h3>{t("Account security")}</h3></div>
          </div>
          <Link className={styles.liveAction} href="/dashboard/account">{t("Password")}</Link>
        </article>

      </section>
    </>
  );
}
