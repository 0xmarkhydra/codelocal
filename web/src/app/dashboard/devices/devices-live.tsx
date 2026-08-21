"use client";

import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource } from "@/lib/contracts/resources";
import { DashboardListControls, dashboardPageSize } from "../dashboard-list-controls";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
import { formatDashboardTime } from "../dashboard-format";
import { DashboardIcon } from "../dashboard-icon";
import visual from "../visual-dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function statusLabel(status: "online" | "offline" | "revoked") { return status[0].toUpperCase() + status.slice(1); }

export function LiveDevices() {
  const { state, retry } = useDashboardResource("/api/v1/devices", isDevicesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  if (state.kind !== "ready") return <DashboardResourceFeedback label="Devices" {...(state.kind === "error" ? { kind:"error" as const, message:state.message, onRetry:retry } : { kind:state.kind })} />;
  const resource = state.value;
  const needle = query.trim().toLowerCase();
  const filtered = needle ? resource.items.filter((d)=>`${d.deviceName} ${d.deviceId}`.toLowerCase().includes(needle)) : resource.items;
  const totalPages = Math.max(1, Math.ceil(filtered.length / dashboardPageSize));
  const currentPage = Math.min(page,totalPages);
  const visible = filtered.slice((currentPage-1)*dashboardPageSize,currentPage*dashboardPageSize);
  return <div className={visual.shell} aria-live="polite">
    <section className={`${visual.panel} ${visual.network}`} aria-label="Paired device network">
      {[0,1,2,3].map((i)=><span className={visual.beam} data-i={i} key={i} />)}<div className={visual.hub}><DashboardIcon name="graph" size={32}/></div>
      <div className={visual.orbit} data-i="0"><DashboardIcon name="monitor"/></div><div className={visual.orbit} data-i="1"><DashboardIcon name="monitor"/></div><div className={visual.orbit} data-i="2"><DashboardIcon name="layers"/></div><div className={visual.orbit} data-i="3"><DashboardIcon name="activity"/></div>
    </section>
    <section className={visual.panel}><div className={visual.toolbar}><DashboardListControls query={query} onQueryChange={setQuery} page={currentPage} totalPages={totalPages} totalResults={filtered.length} onPageChange={setPage} placeholder="Search device" /></div><div className={visual.rows}>{visible.length===0?<p>{query?"No matches":"No devices"}</p>:visible.map((d)=><article className={visual.row} key={d.deviceId}><span className={visual.rowIcon}><DashboardIcon name="monitor"/></span><div className={visual.rowText}><strong>{d.deviceName}</strong><span>{formatDashboardTime(d.lastSeenAt)}</span></div><span className={visual.state} data-state={d.status}>{statusLabel(d.status)}</span><div className={visual.rowActions}><ResourceMutationButton endpoint={`/api/v1/devices/${encodeURIComponent(d.deviceId)}/revoke`} csrf={csrf} label="×" confirmMessage={`Revoke ${d.deviceName}? This disconnects its CodeLocal credential but does not delete local files.`} disabled={d.status==="revoked"} onSuccess={retry}/></div></article>)}</div></section>
    <section className={visual.stats}><article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="monitor"/></span><div><strong>{resource.summary.paired}</strong><span>Paired</span></div></article><article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="check"/></span><div><strong>{resource.summary.online}</strong><span>Online</span></div></article><article className={visual.stat}><span className={visual.statIcon}><DashboardIcon name="shield"/></span><div><strong>{resource.summary.revoked}</strong><span>Revoked</span></div></article></section>
  </div>;
}
