"use client";

import { FormEvent, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { isCodeGraphResource } from "@/lib/contracts/code-graph";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import visual from "../visual-dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { CodeGraphView } from "./code-graph-view";

function workspaceKey(deviceId: string, workspaceId: string) { return `${encodeURIComponent(deviceId)}|${encodeURIComponent(workspaceId)}`; }

export function LiveCodeGraph() {
  const searchParams = useSearchParams();
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const requestedDeviceId = searchParams.get("deviceId") || "";
  const requestedWorkspaceId = searchParams.get("workspaceId") || "";
  const requestedWorkspaceKey = requestedDeviceId && requestedWorkspaceId ? workspaceKey(requestedDeviceId, requestedWorkspaceId) : "";
  const [manualWorkspaceKey, setManualWorkspaceKey] = useState<string | null>(null);
  const [repositoryPath, setRepositoryPath] = useState("");
  const [view, setView] = useState<"architecture" | "files">("architecture");
  const [depth, setDepth] = useState(1);
  const [queryInput, setQueryInput] = useState("");
  const [query, setQuery] = useState("");

  const workspaceItems = workspaces.state.kind === "ready" ? workspaces.state.value.items : [];
  const selectedWorkspaceKey = manualWorkspaceKey ?? requestedWorkspaceKey;
  const selectedWorkspace = workspaceItems.find((w)=>workspaceKey(w.deviceId,w.workspaceId)===selectedWorkspaceKey) ?? workspaceItems[0];
  const graphURL = useMemo(()=>{
    const params = new URLSearchParams({ view, depth:String(depth) });
    if (selectedWorkspace) { params.set("deviceId", selectedWorkspace.deviceId); params.set("workspaceId", selectedWorkspace.workspaceId); }
    if (repositoryPath) params.set("repositoryPath", repositoryPath);
    if (query) params.set("symbol", query);
    return `/api/v1/code/graph?${params.toString()}`;
  },[depth,query,repositoryPath,selectedWorkspace,view]);
  const graph = useDashboardResource(graphURL, isCodeGraphResource);
  function submitQuery(event:FormEvent<HTMLFormElement>){ event.preventDefault(); setQuery(queryInput.trim().slice(0,160)); }

  if (workspaces.state.kind !== "ready") return <DashboardResourceFeedback label="Code Graph" {...(workspaces.state.kind === "error" ? {kind:"error" as const,message:workspaces.state.message,onRetry:workspaces.retry}:{kind:workspaces.state.kind})}/>;
  if (graph.state.kind !== "ready") return <DashboardResourceFeedback label="Code Graph" {...(graph.state.kind === "error" ? {kind:"error" as const,message:graph.state.message,onRetry:graph.retry}:{kind:graph.state.kind})}/>;

  const value = graph.state.value;
  const repositoryValue = repositoryPath || value.repositories[0]?.path || "";
  return <div className={visual.shell}>
    <section className={styles.codeGraphControls}>
      <label><span>Checkout</span><select value={selectedWorkspace?workspaceKey(selectedWorkspace.deviceId,selectedWorkspace.workspaceId):""} disabled={workspaceItems.length===0} onChange={(event)=>{setManualWorkspaceKey(event.target.value);setRepositoryPath("");setQuery("");setQueryInput("");}}>{workspaceItems.length===0&&<option value="">No checkout</option>}{workspaceItems.map((w)=>{const key=workspaceKey(w.deviceId,w.workspaceId);return <option value={key} key={key}>{w.workspaceName}</option>;})}</select></label>
      <label><span>Repo</span><select value={repositoryValue} disabled={value.repositories.length===0} onChange={(event)=>setRepositoryPath(event.target.value)}>{value.repositories.length===0&&<option value="">No snapshot</option>}{value.repositories.map((r)=><option value={r.path} key={r.path}>{r.path==="."?"Primary":r.path}</option>)}</select></label>
      <label><span>View</span><select value={view} onChange={(event)=>setView(event.target.value==="files"?"files":"architecture")}><option value="architecture">Architecture</option><option value="files">Files</option></select></label>
      <label><span>Depth</span><select value={String(depth)} onChange={(event)=>setDepth(Math.min(3,Math.max(1,Number(event.target.value))))}><option value="1">1</option><option value="2">2</option><option value="3">3</option></select></label>
      <form className={styles.codeGraphSearchForm} onSubmit={submitQuery}><label><span>Search</span><input value={queryInput} maxLength={160} placeholder="Symbol / file" onChange={(event)=>setQueryInput(event.target.value)}/></label><button type="submit">↵</button></form>
    </section>
    {value.state !== "current" ? <section className={styles.codeGraphState} data-state={value.state}><h2>{value.state.replaceAll("_"," ")}</h2></section> : <>
      {value.impact && <section className={visual.impactStrip} data-risk={value.impact.risk}><div><strong>{value.impact.directCallers}</strong><span>Callers</span></div><div><strong>{value.impact.directCallees}</strong><span>Callees</span></div><div><strong>{value.impact.affectedFiles}</strong><span>Files</span></div><div><strong>{value.impact.risk}</strong><span>Risk</span></div></section>}
      <div className={visual.graphPage}><CodeGraphView key={`${graphURL}:${value.selectedId??"none"}:${value.nodes.length}`} graph={value}/></div>
    </>}
  </div>;
}
