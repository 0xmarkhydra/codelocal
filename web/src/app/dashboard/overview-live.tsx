"use client";

import Link from "next/link";
import { isDashboardOverview } from "@/lib/contracts/dashboard";
import { AppIcon, type AppIconName } from "./app-icon";
import { DashboardResourceFeedback } from "./dashboard-resource-feedback";
import styles from "./overview.module.css";
import { useDashboardResource } from "./use-dashboard-resource";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";

const destinations: {
  icon: AppIconName;
  title: MessageKey;
  description: MessageKey;
  href: string;
}[] = [
  {
    icon: "connection",
    title: "Connect AI",
    description: "Set up MCP for your client",
    href: "/dashboard/connect",
  },
  {
    icon: "skill",
    title: "Explore skills",
    description: "Extend your AI capabilities",
    href: "/dashboard/skills",
  },
  {
    icon: "shield",
    title: "Review security",
    description: "Manage sessions and access",
    href: "/dashboard/security",
  },
];

export function LiveOverview() {
  const { locale, t } = useTranslations();
  const number = new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: 1 });
  const { state, retry } = useDashboardResource(
    "/api/v1/dashboard/overview",
    isDashboardOverview,
  );
  if (state.kind !== "ready")
    return (
      <DashboardResourceFeedback
        label={t("Overview")}
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  const overview = state.value;
  return (
    <div className={styles.overview}>
      <section className={styles.metrics} aria-label={t("Account metrics")}>
        <Metric
          icon="folder"
          label={t("Projects")}
          value={number.format(overview.workspaces.total)}
          detail={t("{count} active", { count: overview.workspaces.active })}
          tone="purple"
          href="/dashboard/workspaces"
        />
        <Metric
          icon="device"
          label={t("Online devices")}
          value={number.format(overview.devices.online)}
          detail={t("{count} paired devices", { count: overview.devices.paired })}
          tone="cyan"
          href="/dashboard/devices"
        />
        <Metric
          icon="connection"
          label={t("MCP calls / 24 hours")}
          value={
            overview.usage.available
              ? number.format(overview.usage.last24h.calls)
              : "—"
          }
          detail={
            overview.usage.available
              ? t("Recorded calls")
              : t("No usage data yet")
          }
          tone="orange"
          href="/dashboard/usage"
        />
        <Metric
          icon="usage"
          label={t("Tokens / 24 hours")}
          value={
            overview.usage.available
              ? number.format(overview.usage.last24h.totalTokensEstimated)
              : "—"
          }
          detail={
            overview.usage.available
              ? t("Estimated from MCP activity")
              : t("No usage data yet")
          }
          tone="pink"
          href="/dashboard/usage"
        />
      </section>
      <div className={styles.workspaceLayout}>
        <section className={styles.projects}>
          <header className={styles.panelHead}>
            <div>
              <h2>{t("Recent projects")}</h2>
              <span>{t("Workspaces connected to your account")}</span>
            </div>
            <Link href="/dashboard/workspaces">
              {t("View all")} <span aria-hidden="true">↗</span>
            </Link>
          </header>
          {overview.workspaces.recent.length === 0 ? (
            <div className={styles.empty}>
              <AppIcon name="folder" size={30} />
              <strong>{t("Your first project starts here.")}</strong>
              <p>
                {t("Run {command} in your project directory to connect your workspace.", { command: "codelocal ." })}
              </p>
              <Link href="/dashboard/connect">
                {t("Connection guide")} <span aria-hidden="true">↗</span>
              </Link>
            </div>
          ) : (
            <div className={styles.workspaceList}>
              {overview.workspaces.recent.slice(0, 4).map((workspace) => (
                <Link
                  className={styles.workspaceRow}
                  href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}
                  key={`${workspace.deviceId}:${workspace.workspaceId}`}
                >
                  <span className={styles.folderIcon}>
                    <AppIcon name="folder" size={20} />
                  </span>
                  <div>
                    <strong>{workspace.workspaceName}</strong>
                    <small>{workspace.deviceName}</small>
                  </div>
                  <span
                    className={styles.workspaceState}
                    data-state={workspace.status}
                  >
                    <i />
                    {workspace.status === "active"
                      ? t("Online")
                      : workspace.status === "sleeping"
                        ? t("Sleeping")
                        : t("Offline")}
                  </span>
                  <AppIcon name="chevron-right" size={14} />
                </Link>
              ))}
            </div>
          )}
          <div className={styles.projectsFoot}>
            <AppIcon name="shield" size={13} /> {t("Only workspaces you can access are shown.")}
          </div>
        </section>
        <section className={styles.quickAccess}>
          <header className={styles.panelHead}>
            <div>
              <h2>{t("Next steps")}</h2>
              <span>{t("Set up your workspace")}</span>
            </div>
            <AppIcon name="target" size={18} />
          </header>
          <div className={styles.destinations}>
            {destinations.map((item) => (
              <Link href={item.href} key={item.href}>
                <span className={styles.destinationIcon}>
                  <AppIcon name={item.icon} size={18} />
                </span>
                <div>
                  <strong>{t(item.title)}</strong>
                  <small>{t(item.description)}</small>
                </div>
                <AppIcon name="chevron-right" size={14} />
              </Link>
            ))}
          </div>
        </section>
      </div>
      <div className={styles.dataNote}>
        <span>{t("Latest loaded data · MCP tokens are estimates")}</span>
        <button type="button" onClick={retry}>
          <AppIcon name="refresh" size={13} /> {t("Refresh")}
        </button>
      </div>
    </div>
  );
}

function Metric({
  icon,
  label,
  value,
  detail,
  tone,
  href,
}: {
  icon: AppIconName;
  label: string;
  value: string;
  detail: string;
  tone: string;
  href: string;
}) {
  return (
    <Link className={styles.metric} data-tone={tone} href={href}>
      <span className={styles.metricLabel}>
        <span>{label}</span>
        <AppIcon name={icon} size={17} />
      </span>
      <strong>{value}</strong>
      <span className={styles.metricDetail}>
        {detail}
        <span aria-hidden="true">↗</span>
      </span>
    </Link>
  );
}
