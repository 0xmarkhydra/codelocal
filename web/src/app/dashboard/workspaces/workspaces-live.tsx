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
    case "active": return "Trực tuyến";
    case "sleeping": return "Đang nghỉ";
    default: return "Ngoại tuyến";
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
          <span data-state="active"><i />{resource.summary.active} trực tuyến</span>
          {(resource.summary.sleeping + resource.summary.offline) > 0 ? <span data-state="sleeping"><i />{resource.summary.sleeping + resource.summary.offline} đang nghỉ</span> : null}
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
              <Link
                className={styles.workspaceChatLink}
                href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}
                aria-label={`Mở Code Graph của dự án ${workspace.workspaceName}`}
              >
                <span className={styles.workspaceFolderLarge} data-state={workspace.status} aria-hidden="true">
                  <AppIcon name="folder" size={25} />
                  <i />
                </span>
                <span className={styles.workspaceCardBody}>
                  <h2>{workspace.workspaceName}</h2>
                  <span>{workspace.deviceName} · {statusLabel(workspace.status)}</span>
                  <small>Cập nhật {formatDashboardTime(workspace.lastSeenAt)}</small>
                </span>
                <span className={styles.workspaceChatAction}>Code Graph <AppIcon name="chevron-right" size={14} /></span>
              </Link>
              <div className={styles.workspaceCardActions}>
                <details className={styles.workspaceOverflow}>
                  <summary aria-label={`Thêm thao tác cho ${workspace.workspaceName}`}><span aria-hidden="true">•••</span></summary>
                  <div>
                    <Link className={styles.secondaryCardAction} href="/dashboard/knowledge">Project Brain</Link>
                    <ResourceMutationButton
                      endpoint={`/api/v1/workspaces/${encodeURIComponent(workspace.deviceId)}/${encodeURIComponent(workspace.workspaceId)}/remove`}
                      csrf={csrf}
                      label="Xóa"
                      confirmMessage={`Xóa ${workspace.workspaceName} khỏi CodeLocal? Dự án và tệp vẫn được giữ nguyên.`}
                      disabled={!workspace.runtimeOnline}
                      onSuccess={retry}
                    />
                  </div>
                </details>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
