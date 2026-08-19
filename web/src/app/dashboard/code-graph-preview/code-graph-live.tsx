"use client";

import { FormEvent, useMemo, useState } from "react";
import { isCodeGraphResource } from "@/lib/contracts/code-graph";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { CodeGraphView } from "./code-graph-view";

function stateCopy(state: string) {
  switch (state) {
    case "no_workspace": return ["No authorized checkout yet", "Run codelocal . inside a project first. Code Graph only queries explicitly authorized workspaces."] as const;
    case "offline": return ["Local runtime offline", "Start CodeLocal on the selected device. The cloud service does not keep a full source graph as a fallback."] as const;
    case "unsupported": return ["Runtime update required", "This checkout is connected through an older runtime that does not expose bounded Code Graph queries."] as const;
    default: return ["Code Graph unavailable", "The selected local runtime could not provide a bounded graph right now. Project Brain remains independent."] as const;
  }
}

export function LiveCodeGraph() {
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const [workspaceIndex, setWorkspaceIndex] = useState(0);
  const [repositoryPath, setRepositoryPath] = useState("");
  const [view, setView] = useState<"architecture" | "files">("architecture");
  const [depth, setDepth] = useState(1);
  const [queryInput, setQueryInput] = useState("");
  const [query, setQuery] = useState("");

  const selectedWorkspace = workspaces.state.kind === "ready" ? workspaces.state.value.items[workspaceIndex] ?? workspaces.state.value.items[0] : undefined;
  const graphURL = useMemo(() => {
    const params = new URLSearchParams({ view, depth: String(depth) });
    if (selectedWorkspace) {
      params.set("deviceId", selectedWorkspace.deviceId);
      params.set("workspaceId", selectedWorkspace.workspaceId);
    }
    if (repositoryPath) params.set("repositoryPath", repositoryPath);
    if (query) params.set("symbol", query);
    return `/api/v1/code/graph?${params.toString()}`;
  }, [depth, query, repositoryPath, selectedWorkspace, view]);
  const graph = useDashboardResource(graphURL, isCodeGraphResource);

  function submitQuery(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setQuery(queryInput.trim().slice(0, 160));
  }

  if (workspaces.state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Code Graph checkout catalog"
        {...(workspaces.state.kind === "error"
          ? { kind: "error" as const, message: workspaces.state.message, onRetry: workspaces.retry }
          : { kind: workspaces.state.kind })}
      />
    );
  }

  if (graph.state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Code Graph"
        {...(graph.state.kind === "error"
          ? { kind: "error" as const, message: graph.state.message, onRetry: graph.retry }
          : { kind: graph.state.kind })}
      />
    );
  }

  const value = graph.state.value;
  const [stateTitle, stateDescription] = stateCopy(value.state);
  const repositoryValue = repositoryPath || value.repositories[0]?.path || "";

  return (
    <>
      <section className={styles.codeGraphControls}>
        <label>
          <span>Checkout</span>
          <select
            value={String(Math.min(workspaceIndex, Math.max(0, workspaces.state.value.items.length - 1)))}
            onChange={(event) => {
              setWorkspaceIndex(Number(event.target.value));
              setRepositoryPath("");
              setQuery("");
              setQueryInput("");
            }}
          >
            {workspaces.state.value.items.map((workspace, index) => (
              <option value={String(index)} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
                {workspace.workspaceName} · {workspace.deviceName} · {workspace.runtimeOnline ? "online" : workspace.status}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Repository</span>
          <select value={repositoryValue} disabled={value.repositories.length === 0} onChange={(event) => setRepositoryPath(event.target.value)}>
            {value.repositories.length === 0 && <option value="">No snapshot</option>}
            {value.repositories.map((repository) => (
              <option value={repository.path} key={repository.path}>
                {repository.path === "." ? "Primary repository" : repository.path}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>View</span>
          <select value={view} onChange={(event) => setView(event.target.value === "files" ? "files" : "architecture")}>
            <option value="architecture">Architecture</option>
            <option value="files">Files</option>
          </select>
        </label>
        <label>
          <span>Depth</span>
          <select value={String(depth)} onChange={(event) => setDepth(Math.min(3, Math.max(1, Number(event.target.value))))}>
            <option value="1">Depth 1</option>
            <option value="2">Depth 2</option>
            <option value="3">Depth 3</option>
          </select>
        </label>
        <form className={styles.codeGraphSearchForm} onSubmit={submitQuery}>
          <label>
            <span>Symbol</span>
            <input value={queryInput} maxLength={160} placeholder="Function, method, file or symbol…" onChange={(event) => setQueryInput(event.target.value)} />
          </label>
          <button type="submit">Inspect</button>
        </form>
      </section>

      {value.state !== "current" ? (
        <section className={styles.codeGraphState} data-state={value.state}>
          <span className={styles.eyebrow}>{value.state.replaceAll("_", " ")}</span>
          <h2>{stateTitle}</h2>
          <p>{stateDescription}</p>
        </section>
      ) : (
        <>
          <section className={styles.codeGraphState} data-state="current">
            <div>
              <span className={styles.eyebrow}>{value.status === "ambiguous" ? "Ambiguous query" : "Current local snapshot"}</span>
              <h2>{value.status === "ambiguous" ? "Multiple symbols match. CodeLocal did not guess." : `${value.context?.workspaceName ?? "Checkout"} · bounded live graph`}</h2>
              <p>
                {value.repositories[0]
                  ? `${repositoryValue || "."} · ${value.repositories.find((item) => item.path === repositoryValue)?.branch || "detached"} · ${value.nodes.length} visible nodes`
                  : `${value.nodes.length} visible nodes · ${value.edges.length} relationships`}
              </p>
            </div>
            <span className={styles.liveBadge}>{value.truncated ? "BOUNDED · TRUNCATED" : "BOUNDED · CURRENT"}</span>
          </section>

          {value.impact && (
            <section className={styles.codeImpact} data-risk={value.impact.risk}>
              <div>
                <span className={styles.eyebrow}>Impact analysis · {value.impact.risk}</span>
                <strong>{value.impact.directCallers} direct callers · {value.impact.potentialCallers} potential upstream callers · {value.impact.affectedFiles} files</strong>
                <p>{value.impact.truncated ? "The safety bound was reached, so impact remains conservative." : "Impact is computed only from this visible call neighborhood."}</p>
              </div>
              <dl>
                <div><dt>Callees</dt><dd>{value.impact.directCallees}</dd></div>
                <div><dt>Semantic</dt><dd>{value.impact.semanticEdges}/{value.impact.evidenceEdges}</dd></div>
                <div><dt>Confidence</dt><dd>{Math.round(value.impact.averageConfidence * 100)}%</dd></div>
              </dl>
            </section>
          )}

          <CodeGraphView key={`${graphURL}:${value.selectedId ?? "none"}:${value.nodes.length}`} graph={value} />
        </>
      )}
    </>
  );
}
