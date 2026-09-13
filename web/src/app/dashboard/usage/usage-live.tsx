"use client";

import { isUsageResource, type ReportedUsageWindow, type UsageWindow } from "@/lib/contracts/usage";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import usageStyles from "./usage-live.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { useTranslations } from "@/lib/i18n/provider";

const windows = [
  { key: "last1h", label: "Last hour" },
  { key: "last24h", label: "Last 24 hours" },
  { key: "last30d", label: "Last 30 days" },
  { key: "allTime", label: "All time" },
] as const;

function EstimatedBreakdown({ label, value }: { label: string; value: UsageWindow }) {
  const { locale, t } = useTranslations();
  const exactNumber = new Intl.NumberFormat(locale);
  return <article className={styles.usageRow}>
    <div><strong>{label}</strong><span>{t("{count} tool calls", { count: value.calls })}</span></div>
    <dl>
      <div><dt>{t("Input")}</dt><dd>{exactNumber.format(value.inputTokensEstimated)}</dd></div>
      <div><dt>{t("Output")}</dt><dd>{exactNumber.format(value.outputTokensEstimated)}</dd></div>
      <div><dt>{t("Total")}</dt><dd>{exactNumber.format(value.totalTokensEstimated)}</dd></div>
    </dl>
  </article>;
}

function ReportedBreakdown({ label, value }: { label: string; value: ReportedUsageWindow }) {
  const { locale, t } = useTranslations();
  const exactNumber = new Intl.NumberFormat(locale);
  return <article className={styles.usageRow}>
    <div><strong>{label}</strong><span>{t("{count} reported turns", { count: value.turns })}</span></div>
    <dl>
      <div><dt>{t("Input")}</dt><dd>{exactNumber.format(value.inputTokens)}</dd></div>
      <div><dt>{t("Output")}</dt><dd>{exactNumber.format(value.outputTokens)}</dd></div>
      <div><dt>{t("Total")}</dt><dd>{exactNumber.format(value.totalTokens)}</dd></div>
    </dl>
  </article>;
}

export function LiveUsage() {
  const { locale, t } = useTranslations();
  const compactNumber = new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: 1 });
  const { state, retry } = useDashboardResource("/api/v1/usage", isUsageResource);
  if (state.kind !== "ready") {
    return <DashboardResourceFeedback label={t("Usage")} {...(state.kind === "error"
      ? { kind: "error" as const, message: state.message, onRetry: retry }
      : { kind: state.kind })} />;
  }

  const { webChat, mcp } = state.value;
  const combinedTokens = (key: typeof windows[number]["key"]) => webChat[key].totalTokens + mcp[key].totalTokensEstimated;
  return <section className={styles.livePanel} aria-live="polite">
    <div className={styles.liveHead}>
      <div><span className={styles.eyebrow}>Web Chat + MCP</span></div>
      <span className={styles.liveBadge}>{t("Latest snapshot")}</span>
    </div>

    <div className={usageStyles.usageSummary}>
      <article className={usageStyles.primaryMetric}>
        <span>{t("All time")}</span>
        <strong>{compactNumber.format(combinedTokens("allTime"))}</strong>
      </article>
      <div className={usageStyles.recentMetrics}>
        {windows.slice(0, -1).map(({ key, label }) => <article className={usageStyles.recentMetric} key={key}>
          <span>{t(label)}</span>
          <strong>{compactNumber.format(combinedTokens(key))}</strong>
        </article>)}
      </div>
    </div>

    <div className={`${styles.liveHead} ${usageStyles.sectionHead}`}><div><span className={styles.eyebrow}>Web Chat</span><p>{t("Provider-reported tokens")}</p></div></div>
    <div className={styles.usageList}>
      {windows.map(({ key, label }) => <ReportedBreakdown key={key} label={t(label)} value={webChat[key]} />)}
    </div>

    <div className={styles.liveHead}><div><span className={styles.eyebrow}>MCP</span><p>{t("Payload-estimated tokens")}</p></div></div>
    <div className={styles.usageList}>
      {windows.map(({ key, label }) => <EstimatedBreakdown key={key} label={t(label)} value={mcp[key]} />)}
    </div>
  </section>;
}
