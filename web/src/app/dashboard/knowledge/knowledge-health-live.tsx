"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import type { KnowledgeCollective, KnowledgeHealthResource, KnowledgeIndexHealth } from "@/lib/contracts/knowledge-health";
import { isKnowledgeHealthResource } from "@/lib/contracts/knowledge-health";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey, MessageValues } from "@/lib/i18n/messages";
import type { Locale } from "@/lib/i18n/locale";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function compactAge(milliseconds: number, locale: Locale) {
  const unit = (value: number, unit: string) => new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "short" }).format(value);
  const seconds = Math.max(0, Math.floor(milliseconds / 1000));
  if (seconds < 60) return unit(seconds, "second");
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return unit(minutes, "minute");
  const hours = Math.floor(minutes / 60);
  return new Intl.ListFormat(locale, { style: "short", type: "unit" }).format([unit(hours, "hour"), unit(minutes % 60, "minute")]);
}

function healthState(available: boolean, status: string, t: (key: MessageKey) => string) {
  if (!available) return t("Unavailable");
  switch (status.trim()) {
    case "": return t("Available");
    case "healthy": return t("Healthy");
    case "degraded": return t("Degraded");
    case "critical": return t("Critical");
    case "current": return t("Current");
    case "stale": return t("Stale");
    case "missing": return t("Missing");
    default: return status;
  }
}

function IndexHealthCard({ title, value }: { title: string; value: KnowledgeIndexHealth }) {
  const { locale, t } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  return (
    <article className={styles.healthCard}>
      <div className={styles.healthCardHead}>
        <span>{title}</span>
        <b>{healthState(value.available, value.status, t)}</b>
      </div>
      <dl className={styles.healthMetrics}>
        <div><dt>{t("Projects")}</dt><dd>{number.format(value.projectCount)}</dd></div>
        <div><dt>{t("Current")}</dt><dd>{number.format(value.currentProjects)}</dd></div>
        <div><dt>{t("Stale")}</dt><dd>{number.format(value.staleProjects)}</dd></div>
        <div><dt>{t("Missing")}</dt><dd>{number.format(value.missingProjects)}</dd></div>
        <div><dt>{t("Max lag")}</dt><dd>{compactAge(value.maxLagMs, locale)}</dd></div>
      </dl>
    </article>
  );
}

function CollectiveControls({ csrf, value }: { csrf: string; value: KnowledgeCollective }) {
  const { locale, t } = useTranslations();
  const percent = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 });
  const router = useRouter();
  const [contributionEnabled, setContributionEnabled] = useState(value.contributionEnabled);
  const [suggestionsEnabled, setSuggestionsEnabled] = useState(value.suggestionsEnabled);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<{ key: MessageKey; values?: MessageValues } | null>(null);

  async function save() {
    if (saving || !value.available) return;
    setSaving(true);
    setNotice(null);
    const body = new URLSearchParams({
      csrf,
      contributionEnabled: contributionEnabled ? "1" : "0",
      suggestionsEnabled: suggestionsEnabled ? "1" : "0",
    });
    try {
      const response = await fetch("/api/collective/preferences", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" },
        body,
      });
      if (response.status === 401) {
        router.push("/login?next=/dashboard/knowledge");
        return;
      }
      if (!response.ok) {
        setNotice(response.status === 403
          ? { key: "Security token expired. Reload this page before saving again." }
          : { key: "Save failed with {status}.", values: { status: String(response.status) } });
        return;
      }
      setNotice({ key: "Collective settings saved." });
    } catch {
      setNotice({ key: "Collective settings could not be saved right now." });
    } finally {
      setSaving(false);
    }
  }

  return (
    <article className={styles.collectivePanel}>
      <div className={styles.healthCardHead}>
        <div>
          <span>{t("Collective intelligence")}</span>
          <p>{t("Private opt-in. You can disable contribution at any time; CodeLocal removes your contribution aggregates when you opt out.")}</p>
        </div>
        <b>{t(value.available ? "Available" : "Unavailable")}</b>
      </div>
      <label className={styles.toggleRow}>
        <input
          type="checkbox"
          checked={contributionEnabled}
          disabled={!value.available || saving}
          onChange={(event) => setContributionEnabled(event.target.checked)}
        />
        <span>
          <strong>{t("Contribute anonymized engineering patterns")}</strong>
          <small>{t("Rollout: {status}. Disabling this withdraws your current contribution ledger and aggregates.", { status: t(value.contributionAvailable ? "Available" : "Paused") })}</small>
        </span>
      </label>
      <label className={styles.toggleRow}>
        <input
          type="checkbox"
          checked={suggestionsEnabled}
          disabled={!value.available || saving}
          onChange={(event) => setSuggestionsEnabled(event.target.checked)}
        />
        <span>
          <strong>{t("Receive collective suggestions")}</strong>
          <small>{t("Rollout: {status}. A cohort requires at least {count} independent contributors.", { status: t(value.suggestionsAvailable ? "Available" : "Paused"), count: value.minimumContributors })}</small>
        </span>
      </label>
      <button className={styles.liveAction} type="button" disabled={!value.available || saving} onClick={save}>
        {t(saving ? "Saving…" : "Save collective settings")}
      </button>
      {notice && <p className={styles.controlMessage} role="status">{t(notice.key, notice.values)}</p>}

      {suggestionsEnabled && value.recommendations.length > 0 && (
        <div className={styles.recommendationList}>
          <strong>{t("Privacy-safe cohort signals")}</strong>
          {value.recommendations.map((item, index) => (
            <div key={`${item.taskKind}-${item.executionTool}-${index}`}>
              <span>{item.taskKind.replaceAll("_", " ")} · {t("{rate} mean success", { rate: percent.format(item.meanUserSuccessRate) })}</span>
              <small>{t("{count} contributors · checks {checks} · files {files} · quality {quality}", { count: item.contributorCount, checks: item.checkProfile.join(", ") || t("Verified"), files: item.fileCountBucket, quality: item.qualityBucket })}</small>
            </div>
          ))}
        </div>
      )}
    </article>
  );
}

function HealthContent({ value }: { value: KnowledgeHealthResource }) {
  const { locale, t } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  return (
    <section className={styles.knowledgeHealthSection}>
      <div className={styles.healthGrid}>
        <article className={styles.healthCard}>
          <div className={styles.healthCardHead}>
            <span>{t("Durable learning pipeline")}</span>
            <b>{healthState(value.pipeline.available, value.pipeline.status, t)}</b>
          </div>
          <dl className={styles.healthMetrics}>
            <div><dt>{t("Pending")}</dt><dd>{number.format(value.pipeline.pendingCount)}</dd></div>
            <div><dt>{t("Retrying")}</dt><dd>{number.format(value.pipeline.retryingCount)}</dd></div>
            <div><dt>{t("Dead")}</dt><dd>{number.format(value.pipeline.deadCount)}</dd></div>
            <div><dt>{t("Processed 1h")}</dt><dd>{number.format(value.pipeline.processedLastHour)}</dd></div>
            <div><dt>{t("Oldest active")}</dt><dd>{compactAge(value.pipeline.oldestActiveAgeMs, locale)}</dd></div>
          </dl>
        </article>
        <IndexHealthCard title={t("Canonical Knowledge Graph")} value={value.graphIndex} />
        <IndexHealthCard title={t("Canonical Semantic Index")} value={value.semanticIndex} />
        <article className={styles.healthCard}>
          <div className={styles.healthCardHead}>
            <span>{t("Semantic Hybrid canary")}</span>
            <b>{t(value.canary.available ? (value.canary.attemptsTotal > 0 ? "Observing" : "Collecting") : "Unavailable")}</b>
          </div>
          <dl className={styles.healthMetrics}>
            <div><dt>{t("Attempts")}</dt><dd>{number.format(value.canary.attemptsTotal)}</dd></div>
            <div><dt>{t("Applied")}</dt><dd>{number.format(value.canary.appliedCount)}</dd></div>
            <div><dt>{t("Fallback")}</dt><dd>{number.format(value.canary.deterministicFallbackCount)}</dd></div>
            <div><dt>{t("Errors")}</dt><dd>{number.format(value.canary.errorCount)}</dd></div>
            <div><dt>{t("Timeout")}</dt><dd>{number.format(value.canary.timeoutCount)}</dd></div>
          </dl>
        </article>
      </div>

      <CollectiveControls csrf={value.csrf} value={value.collective} />

      <div className={styles.privacyBoundary}>
        <strong>{t("Collective privacy boundary")}</strong>
        <span>{t("Raw code shared: {value}", { value: t(value.privacy.rawCodeShared ? "Yes" : "No") })}</span>
        <span>{t("Conversations shared: {value}", { value: t(value.privacy.conversationShared ? "Yes" : "No") })}</span>
        <span>{t("Project identity shared: {value}", { value: t(value.privacy.projectIdentityShared ? "Yes" : "No") })}</span>
        <span>{t("Local replay trust shared: {value}", { value: t(value.privacy.localReplayTrustShared ? "Yes" : "No") })}</span>
      </div>
    </section>
  );
}

export function LiveKnowledgeHealth() {
  const { t } = useTranslations();
  const { state, retry } = useDashboardResource("/api/v1/knowledge/health", isKnowledgeHealthResource);
  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label={t("Project Brain health")}
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }
  return (
    <details className={styles.knowledgeHealthDetails}>
      <summary>{t("Health & controls")}</summary>
      <HealthContent value={state.value} />
    </details>
  );
}
