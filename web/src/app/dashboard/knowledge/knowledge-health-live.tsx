"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import type { KnowledgeCollective, KnowledgeHealthResource, KnowledgeIndexHealth } from "@/lib/contracts/knowledge-health";
import { isKnowledgeHealthResource } from "@/lib/contracts/knowledge-health";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function compactAge(milliseconds: number) {
  if (milliseconds <= 0) return "0s";
  const seconds = Math.floor(milliseconds / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${String(minutes % 60).padStart(2, "0")}m`;
}

function healthState(available: boolean, status: string) {
  if (!available) return "Unavailable";
  return status.trim() || "Available";
}

function IndexHealthCard({ title, value }: { title: string; value: KnowledgeIndexHealth }) {
  return (
    <article className={styles.healthCard}>
      <div className={styles.healthCardHead}>
        <span>{title}</span>
        <b>{healthState(value.available, value.status)}</b>
      </div>
      <dl className={styles.healthMetrics}>
        <div><dt>Projects</dt><dd>{value.projectCount}</dd></div>
        <div><dt>Current</dt><dd>{value.currentProjects}</dd></div>
        <div><dt>Stale</dt><dd>{value.staleProjects}</dd></div>
        <div><dt>Missing</dt><dd>{value.missingProjects}</dd></div>
        <div><dt>Max lag</dt><dd>{compactAge(value.maxLagMs)}</dd></div>
      </dl>
    </article>
  );
}

function CollectiveControls({ csrf, value }: { csrf: string; value: KnowledgeCollective }) {
  const router = useRouter();
  const [contributionEnabled, setContributionEnabled] = useState(value.contributionEnabled);
  const [suggestionsEnabled, setSuggestionsEnabled] = useState(value.suggestionsEnabled);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  async function save() {
    if (saving || !value.available) return;
    setSaving(true);
    setMessage(null);
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
        setMessage(response.status === 403 ? "Security token expired. Reload this page before saving again." : `Save failed with ${response.status}.`);
        return;
      }
      setMessage("Collective settings saved.");
    } catch {
      setMessage("Collective settings could not be saved right now.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <article className={styles.collectivePanel}>
      <div className={styles.healthCardHead}>
        <div>
          <span>Collective intelligence</span>
          <p>Private opt-in. You can disable contribution at any time; CodeLocal removes your contribution aggregates when you opt out.</p>
        </div>
        <b>{value.available ? "Available" : "Unavailable"}</b>
      </div>
      <label className={styles.toggleRow}>
        <input
          type="checkbox"
          checked={contributionEnabled}
          disabled={!value.available || saving}
          onChange={(event) => setContributionEnabled(event.target.checked)}
        />
        <span>
          <strong>Contribute anonymized engineering patterns</strong>
          <small>Rollout: {value.contributionAvailable ? "available" : "paused"}. Disabling this withdraws your current contribution ledger and aggregates.</small>
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
          <strong>Receive collective suggestions</strong>
          <small>Rollout: {value.suggestionsAvailable ? "available" : "paused"}. A cohort requires at least {value.minimumContributors} independent contributors.</small>
        </span>
      </label>
      <button className={styles.liveAction} type="button" disabled={!value.available || saving} onClick={save}>
        {saving ? "Saving…" : "Save collective settings"}
      </button>
      {message && <p className={styles.controlMessage} role="status">{message}</p>}

      {suggestionsEnabled && value.recommendations.length > 0 && (
        <div className={styles.recommendationList}>
          <strong>Privacy-safe cohort signals</strong>
          {value.recommendations.map((item, index) => (
            <div key={`${item.taskKind}-${item.executionTool}-${index}`}>
              <span>{item.taskKind.replaceAll("_", " ")} · {Math.round(item.meanUserSuccessRate * 100)}% mean success</span>
              <small>{item.contributorCount} contributors · checks {item.checkProfile.join(", ") || "verified"} · files {item.fileCountBucket} · quality {item.qualityBucket}</small>
            </div>
          ))}
        </div>
      )}
    </article>
  );
}

function HealthContent({ value }: { value: KnowledgeHealthResource }) {
  return (
    <section className={styles.knowledgeHealthSection}>
      <div className={styles.healthSectionHead}>
        <div>
          <span className={styles.eyebrow}>Project Brain health & controls</span>
          <h2>Derived indexes stay observable without exposing project content.</h2>
          <p>These signals summarize Project Brain health. If a derived index is stale or unavailable, CodeLocal continues using the durable project knowledge it already trusts.</p>
        </div>
        <span className={styles.liveBadge}>GO AUTHORITY</span>
      </div>

      <div className={styles.healthGrid}>
        <article className={styles.healthCard}>
          <div className={styles.healthCardHead}>
            <span>Durable learning pipeline</span>
            <b>{healthState(value.pipeline.available, value.pipeline.status)}</b>
          </div>
          <dl className={styles.healthMetrics}>
            <div><dt>Pending</dt><dd>{value.pipeline.pendingCount}</dd></div>
            <div><dt>Retrying</dt><dd>{value.pipeline.retryingCount}</dd></div>
            <div><dt>Dead</dt><dd>{value.pipeline.deadCount}</dd></div>
            <div><dt>Processed 1h</dt><dd>{value.pipeline.processedLastHour}</dd></div>
            <div><dt>Oldest active</dt><dd>{compactAge(value.pipeline.oldestActiveAgeMs)}</dd></div>
          </dl>
        </article>
        <IndexHealthCard title="Canonical Knowledge Graph" value={value.graphIndex} />
        <IndexHealthCard title="Canonical Semantic Index" value={value.semanticIndex} />
        <article className={styles.healthCard}>
          <div className={styles.healthCardHead}>
            <span>Semantic Hybrid canary</span>
            <b>{value.canary.available ? (value.canary.attemptsTotal > 0 ? "Observing" : "Collecting") : "Unavailable"}</b>
          </div>
          <dl className={styles.healthMetrics}>
            <div><dt>Attempts</dt><dd>{value.canary.attemptsTotal}</dd></div>
            <div><dt>Applied</dt><dd>{value.canary.appliedCount}</dd></div>
            <div><dt>Fallback</dt><dd>{value.canary.deterministicFallbackCount}</dd></div>
            <div><dt>Errors</dt><dd>{value.canary.errorCount}</dd></div>
            <div><dt>Timeout</dt><dd>{value.canary.timeoutCount}</dd></div>
          </dl>
        </article>
      </div>

      <CollectiveControls csrf={value.csrf} value={value.collective} />

      <div className={styles.privacyBoundary}>
        <strong>Collective privacy boundary</strong>
        <span>Raw code shared: {String(value.privacy.rawCodeShared)}</span>
        <span>Conversations shared: {String(value.privacy.conversationShared)}</span>
        <span>Project identity shared: {String(value.privacy.projectIdentityShared)}</span>
        <span>Local replay trust shared: {String(value.privacy.localReplayTrustShared)}</span>
      </div>
    </section>
  );
}

export function LiveKnowledgeHealth() {
  const { state, retry } = useDashboardResource("/api/v1/knowledge/health", isKnowledgeHealthResource);
  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Project Brain health"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }
  return <HealthContent value={state.value} />;
}
