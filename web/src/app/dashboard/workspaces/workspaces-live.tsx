"use client";

import Link from "next/link";
import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardListControls, dashboardPageSize } from "../dashboard-list-controls";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
import { formatDashboardTime } from "../dashboard-format";
import { DashboardIcon } from "../dashboard-icon";
import visual from "../visual-dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function statusLabel(status: "active" | "sleeping" | "offline") { return status[0].toUpperCase() + status.slice(1); }

export function LiveWorkspaces() {
  const { state, retry } = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  if (state.kind !== "ready") return <DashboardResourceFeedback label="Workspaces" {...(state.kind === "error" ? { kind:"error" as const, message:state.message, onRetry:retry } : { kind:state.kind })} />;
  const resource = state.value;
  const needle = query.trim().toLowerCase();
  const filtered = needle ? resource.items.filter((w)=>`${w.workspaceName} ${w.deviceName} ${w.workspaceId}`.toLowerCase().includes(needle)) : resource.items;
  const totalPages = Math.max(1, Math.ceil(filtered.length / dashboardPageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = filtered.slice((currentPage-1)*dashboardPageSize, currentPage*dashboardPageSize);
  return (
    <div className={visual.shell} aria-live="polite">
      <div className={visual.split}>
        <section className={`${visual.panel} ${visual.stats}`}>
          <article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="grid" /></span><div><strong>{resource.summary.total}</strong><span>All</span></div></article>
          <article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="circle" /></span><div><strong>{resource.summary.active}</strong><span>Active</span></div></article>
          <article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="moon" /></span><div><strong>{resource.summary.sleeping}</strong><span>Sleep</span></div></article>
          <article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="circle" /></span><div><strong>{resource.summary.offline}</strong><span>Offline</span></div></article>
        </section>
        <section className={`${visual.panel} ${visual.visualMini}`} aria-hidden="true">
          {[0,1,2,3].map((i)=><span className={visual.beam} data-i={i} key={i} />)}<div className={visual.hub}><DashboardIcon name="grid" /></div>
          <div className={visual.orbit} data-i="0"><DashboardIcon name="grid" size={15}/></div><div className={visual.orbit} data-i="1"><DashboardIcon name="grid" size={15}/></div><div className={visual.orbit} data-i="2"><DashboardIcon name="monitor" size={15}/></div><div className={visual.orbit} data-i="3"><DashboardIcon name="monitor" size={15}/></div>
        </section>
      </div>
      <section className={visual.panel}>
        <div className={visual.toolbar}><DashboardListControls query={query} onQueryChange={setQuery} page={currentPage} totalPages={totalPages} totalResults={filtered.length} onPageChange={setPage} placeholder="Search workspace" /></div>
        <div className={visual.rows}>{visible.length === 0 ? <p>{query ? "No matches" : "No workspaces"}</p> : visible.map((w)=><article className={visual.row} key={`${w.deviceId}:${w.workspaceId}`}>
          <span className={visual.rowIcon}><DashboardIcon name="grid" /></span>
          <div className={visual.rowText}><strong>{w.workspaceName}</strong><span>{w.deviceName} · {formatDashboardTime(w.lastSeenAt)}</span></div>
          <span className={visual.state} data-state={w.status}>{statusLabel(w.status)}</span>
          <div className={visual.rowActions}><Link href={`/dashboard/code-graph?deviceId=${encodeURIComponent(w.deviceId)}&workspaceId=${encodeURIComponent(w.workspaceId)}`} aria-label="Code Graph" title="Code Graph"><DashboardIcon name="graph" size={15}/></Link><ResourceMutationButton endpoint={`/api/v1/workspaces/${encodeURIComponent(w.deviceId)}/${encodeURIComponent(w.workspaceId)}/remove`} csrf={csrf} label="×" confirmMessage={`Remove ${w.workspaceName} from CodeLocal? The project and files stay untouched.`} disabled={!w.runtimeOnline} onSuccess={retry} /></div>
        </article>)}</div>
      </section>
    </div>
  );
}
