"use client";

import { isKnowledgeGraphResource } from "@/lib/contracts/knowledge";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import visual from "../visual-dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { KnowledgeGraphView } from "./knowledge-graph-view";

export function LiveKnowledgeGraph() {
  const { state, retry } = useDashboardResource("/api/v1/knowledge/graph", isKnowledgeGraphResource);
  if (state.kind !== "ready") return <DashboardResourceFeedback label="Knowledge" {...(state.kind === "error" ? { kind:"error" as const, message:state.message, onRetry:retry } : { kind:state.kind })} />;
  const graph = state.value;
  return <div className={visual.graphPage} aria-live="polite"><div className={visual.graphMeta}><span>{graph.stats.projects ?? 0} projects</span><span>{graph.nodes.length} nodes</span><span>{graph.edges.length} links</span></div><KnowledgeGraphView graph={graph} /></div>;
}
