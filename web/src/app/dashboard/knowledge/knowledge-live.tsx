"use client";

import { isKnowledgeGraphResource } from "@/lib/contracts/knowledge";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { KnowledgeGraphView } from "./knowledge-graph-view";

export function LiveKnowledgeGraph() {
  const { state, retry } = useDashboardResource("/api/v1/knowledge/graph", isKnowledgeGraphResource);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Knowledge Graph"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const graph = state.value;
  return (
    <section className={styles.livePanel} aria-live="polite">
      <div className={styles.liveHead}>
        <div>
          <span className={styles.eyebrow}>Knowledge Graph</span>
          <h2>Durable Project Brain relationships, rendered from a bounded Go graph contract.</h2>
          <p>
            Browser IDs are response-local and repository remotes, device identifiers and source-memory IDs are excluded from this client contract.
          </p>
        </div>
        <span className={styles.liveBadge}>LIVE BACKEND DATA</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}>
          <span>Projects</span>
          <strong>{graph.stats.projects ?? 0}</strong>
          <p>Logical projects represented in this bounded graph response.</p>
        </article>
        <article className={styles.metricCard}>
          <span>Knowledge nodes</span>
          <strong>{graph.nodes.length}</strong>
          <p>{graph.meta.atNodeLimit ? `Response reached the ${graph.meta.nodeLimit}-node safety bound.` : `Below the ${graph.meta.nodeLimit}-node response bound.`}</p>
        </article>
        <article className={styles.metricCard}>
          <span>Relationships</span>
          <strong>{graph.edges.length}</strong>
          <p>Only edges whose endpoints are present in this response are emitted.</p>
        </article>
      </div>

      <KnowledgeGraphView graph={graph} />
    </section>
  );
}
