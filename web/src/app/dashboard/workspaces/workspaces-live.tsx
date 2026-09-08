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
import { useTranslations } from "@/lib/i18n/provider";

function statusLabel(status: "active" | "sleeping" | "offline") {
  switch (status) {
    case "active": return "Online";
    case "sleeping": return "Sleeping";
    default: return "Offline";
  }
}

export function LiveWorkspaces() {
  const { locale, t } = useTranslations();
  const { state, retry } = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);

  if (state.kind !== "ready") {
    return <DashboardResourceFeedback label={t("Projects")} {...(state.kind === "error" ? { kind: "error" as const, message: state.message, onRetry: retry } : { kind: state.kind })} />;
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
        <div className={styles.summaryPills} aria-label={t("Project summary")}>
          <span>{t("{count} projects", { count: resource.summary.total })}</span>
          <span data-state="active"><i />{t("{count} online", { count: resource.summary.active })}</span>
          {(resource.summary.sleeping + resource.summary.offline) > 0 ? <span data-state="sleeping"><i />{t("{count} inactive", { count: resource.summary.sleeping + resource.summary.offline })}</span> : null}
        </div>
        <DashboardListControls
          query={query}
          onQueryChange={(value) => { setQuery(value); setPage(1); }}
          page={currentPage}
          totalPages={totalPages}
          totalResults={filtered.length}
          onPageChange={setPage}
          placeholder={t("Search projects")}
        />
      </div>

      {visible.length === 0 ? (
        <div className={styles.appleEmptyState}>
          <span className={styles.emptyFolderIcon} aria-hidden="true" />
          <strong>{t(query ? "No matching workspaces" : "No workspaces yet")}</strong>
          <p>{query ? t("Try another search.") : t("Run {command} in your project directory to connect your workspace.", { command: "codelocal ." })}</p>
        </div>
      ) : (
        <div className={styles.workspaceGrid}>
          {visible.map((workspace) => (
            <article className={styles.workspaceCard} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
              <Link
                className={styles.workspaceChatLink}
                href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}
                aria-label={t("Open Code Graph for {name}", { name: workspace.workspaceName })}
              >
                <span className={styles.workspaceFolderLarge} data-state={workspace.status} aria-hidden="true">
                  <AppIcon name="folder" size={25} />
                  <i />
                </span>
                <span className={styles.workspaceCardBody}>
                  <h2>{workspace.workspaceName}</h2>
                  <span>{workspace.deviceName} · {t(statusLabel(workspace.status))}</span>
                  <small>{t("Updated {time}", { time: formatDashboardTime(workspace.lastSeenAt, locale) })}</small>
                </span>
                <span className={styles.workspaceChatAction}>Code Graph <AppIcon name="chevron-right" size={14} /></span>
              </Link>
              <div className={styles.workspaceCardActions}>
                <details className={styles.workspaceOverflow}>
                  <summary aria-label={t("More actions for {name}", { name: workspace.workspaceName })}><span aria-hidden="true">•••</span></summary>
                  <div>
                    <Link className={styles.secondaryCardAction} href="/dashboard/knowledge">Project Brain</Link>
                    <ResourceMutationButton
                      endpoint={`/api/v1/workspaces/${encodeURIComponent(workspace.deviceId)}/${encodeURIComponent(workspace.workspaceId)}/remove`}
                      csrf={csrf}
                      label={t("Remove")}
                      confirmMessage={t("Remove {name} from CodeLocal? Your project and files will be preserved.", { name: workspace.workspaceName })}
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
