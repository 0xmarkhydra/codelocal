"use client";

import Link from "next/link";
import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardListControls, dashboardPageSize } from "../dashboard-list-controls";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function statusLabel(status: "active" | "sleeping" | "offline") {
  switch (status) {
    case "active":
      return "Active";
    case "sleeping":
      return "Sleeping";
    default:
      return "Offline";
  }
}

export function LiveWorkspaces() {
  const { state, retry } = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Authorized workspaces"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const resource = state.value;
  const needle = query.trim().toLowerCase();
  const filtered = needle
    ? resource.items.filter((workspace) => (
      `${workspace.workspaceName} ${workspace.workspaceId} ${workspace.deviceName} ${workspace.deviceId}`.toLowerCase().includes(needle)
    ))
    : resource.items;
  const totalPages = Math.max(1, Math.ceil(filtered.length / dashboardPageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = filtered.slice((currentPage - 1) * dashboardPageSize, currentPage * dashboardPageSize);

  return (
    <section className={styles.livePanel} aria-live="polite">
      <div className={styles.liveHead}>
        <div>
          <span className={styles.eyebrow}>Authorized workspaces</span>
          <h2>Project folders visible to CodeLocal.</h2>
          <p>This list comes from the Go workspace catalog and omits local paths, capabilities and routing internals.</p>
        </div>
        <span className={styles.liveBadge}>Live</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}><span>Authorized</span><strong>{resource.summary.total}</strong><p>Folders explicitly granted to CodeLocal.</p></article>
        <article className={styles.metricCard}><span>Active</span><strong>{resource.summary.active}</strong><p>Loaded for an active MCP session.</p></article>
        <article className={styles.metricCard}><span>Sleeping / offline</span><strong>{resource.summary.sleeping}<small> / {resource.summary.offline}</small></strong><p>Authorized without an active heavy runtime.</p></article>
      </div>

      <DashboardListControls
        query={query}
        onQueryChange={setQuery}
        page={currentPage}
        totalPages={totalPages}
        totalResults={filtered.length}
        onPageChange={setPage}
        placeholder="Search workspace, device or ID"
      />

      <div className={styles.resourceList}>
        {visible.length === 0 ? (
          <p className={styles.emptyCopy}>{query ? "No workspaces match your search." : <>No authorized workspace has synced yet. Run <code>codelocal .</code> inside a project to authorize it.</>}</p>
        ) : visible.map((workspace) => (
          <article className={`${styles.resourceRow} ${styles.workspaceResourceRow}`} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
            <span className={styles.workspaceFolderIcon} data-state={workspace.status} aria-hidden="true">
              <svg viewBox="0 0 24 24">
                <path d="M3 6.5A2.5 2.5 0 0 1 5.5 4H10l2 2.5h6.5A2.5 2.5 0 0 1 21 9v7.5a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3z" />
              </svg>
              <i />
            </span>
            <div className={styles.resourceIdentity}>
              <strong>{workspace.workspaceName}</strong>
              <span>{workspace.deviceName} · last seen {formatDashboardTime(workspace.lastSeenAt)}</span>
            </div>
            <div className={styles.resourceActions}>
              <Link
                className={styles.liveAction}
                href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}
              >
                Code Graph
              </Link>
              <span className={styles.resourceStatus}>{statusLabel(workspace.status)}</span>
              <ResourceMutationButton
                endpoint={`/api/v1/workspaces/${encodeURIComponent(workspace.deviceId)}/${encodeURIComponent(workspace.workspaceId)}/remove`}
                csrf={csrf}
                label="Remove access"
                confirmMessage={`Remove ${workspace.workspaceName} from CodeLocal? The project and files stay untouched.`}
                disabled={!workspace.runtimeOnline}
                onSuccess={retry}
              />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
