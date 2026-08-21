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
        <div><span className={styles.eyebrow}>Project Brain</span></div>
        <span className={styles.liveBadge}>Live</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}><span>Projects</span><strong>{graph.stats.projects ?? 0}</strong></article>
        <article className={styles.metricCard}><span>Nodes</span><strong>{graph.nodes.length}</strong></article>
        <article className={styles.metricCard}><span>Links</span><strong>{graph.edges.length}</strong></article>
      </div>

      <KnowledgeGraphView graph={graph} />
    </section>
  );
}
