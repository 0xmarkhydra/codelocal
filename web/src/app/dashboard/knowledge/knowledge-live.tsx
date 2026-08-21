"use client";

import { isKnowledgeGraphResource } from "@/lib/contracts/knowledge";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
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

  return <KnowledgeGraphView graph={state.value} />;
}
