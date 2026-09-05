"use client";

import Link from "next/link";
import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import { DashboardListControls, dashboardPageSize } from "../dashboard-list-controls";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
import { AppIcon } from "../app-icon";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

function statusLabel(status: "active" | "sleeping" | "offline") {
  switch (status) {
    case "active": return "Online";
    case "sleeping": return "Sleeping";
    default: return "Offline";
  }
}

export function LiveWorkspaces() {
  const { state, retry } = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);

  if (state.kind !== "ready") {
    return <DashboardResourceFeedback label="Projects" {...(state.kind === "error" ? { kind: "error" as const, message: state.message, onRetry: retry } : { kind: state.kind })} />;
  }

  const resource = state.value;
  const needle = query.trim().toLowerCase();
  const filtered = needle
    ? resource.items.filter((workspace) => `${workspace.workspaceName} ${workspace.workspaceId} ${workspace.deviceName} ${workspace.deviceId}`.toLowerCase().includes(needle))
    : resource.items;
  const totalPages = Math.max(1, Math.ceil(filtered.length / dashboardPageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = filtered.slice((currentPage - 1) * dashboardPageSize, currentPage * dashboardPageSize);

  return (
    <section className={styles.workspaceSection} aria-live="polite">
      <div className={styles.workspaceToolbar}>
        <div className={styles.summaryPills} aria-label="Project summary">
          <span><strong>{resource.summary.total}</strong> dự án</span>
          <span data-state="active"><i />{resource.summary.active} online</span>
          {(resource.summary.sleeping + resource.summary.offline) > 0 ? <span data-state="sleeping"><i />{resource.summary.sleeping + resource.summary.offline} nghỉ</span> : null}
        </div>
        <DashboardListControls
          query={query}
          onQueryChange={(value) => { setQuery(value); setPage(1); }}
          page={currentPage}
          totalPages={totalPages}
          totalResults={filtered.length}
          onPageChange={setPage}
          placeholder="Tìm project"
        />
      </div>

      {visible.length === 0 ? (
        <div className={styles.appleEmptyState}>
          <span className={styles.emptyFolderIcon} aria-hidden="true" />
          <strong>{query ? "Không tìm thấy workspace" : "Chưa có workspace"}</strong>
          <p>{query ? "Thử một từ khóa khác." : "Chạy codelocal . trong thư mục dự án để thêm workspace."}</p>
        </div>
      ) : (
        <div className={styles.workspaceGrid}>
          {visible.map((workspace) => (
            <article className={styles.workspaceCard} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
              <div className={styles.workspaceCardHead}>
                <span className={styles.workspaceFolderLarge} data-state={workspace.status} aria-hidden="true">
                  <AppIcon name="folder" size={25} />
                </span>
                <span className={styles.statusPill} data-state={workspace.status}><i />{statusLabel(workspace.status)}</span>
              </div>
              <div className={styles.workspaceCardBody}>
                <h2>{workspace.workspaceName}</h2>
                <p>{workspace.deviceName}</p>
                <small>Cập nhật {formatDashboardTime(workspace.lastSeenAt)}</small>
              </div>
              <div className={styles.workspaceCardActions}>
                <Link className={styles.primaryCardAction} href={`/dashboard?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}>Chat với dự án <AppIcon name="chevron-right" size={14} /></Link>
                <Link className={styles.secondaryCardAction} href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}>Brain</Link>
                <ResourceMutationButton
                  endpoint={`/api/v1/workspaces/${encodeURIComponent(workspace.deviceId)}/${encodeURIComponent(workspace.workspaceId)}/remove`}
                  csrf={csrf}
                  label="Remove"
                  confirmMessage={`Remove ${workspace.workspaceName} from CodeLocal? The project and files stay untouched.`}
                  disabled={!workspace.runtimeOnline}
                  onSuccess={retry}
                />
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
