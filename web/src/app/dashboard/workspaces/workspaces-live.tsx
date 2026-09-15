"use client";

import Link from "next/link";
import { useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource, isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { DashboardListControls, dashboardPageSize } from "../dashboard-list-controls";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { ResourceMutationButton } from "../resource-mutation-button";
import { AppIcon } from "../app-icon";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";
import { useTranslations } from "@/lib/i18n/provider";

const videoStudioWorkspaceID = "system-openmontage";

type WorkspaceItem = WorkspacesResource["items"][number];

function statusLabel(status: "active" | "sleeping" | "offline") {
  switch (status) {
    case "active": return "Online";
    case "sleeping": return "Sleeping";
    default: return "Offline";
  }
}

function InstallSystemAppButton({ deviceId, csrf, disabled, onSuccess }: { deviceId: string; csrf?: string; disabled?: boolean; onSuccess: () => void }) {
  const { t } = useTranslations();
  const [state, setState] = useState<"idle" | "installing" | "error">("idle");

  async function install() {
    if (!csrf || disabled || state === "installing") return;
    setState("installing");
    try {
      const response = await fetch(`/api/v1/system-apps/openmontage/${encodeURIComponent(deviceId)}/install`, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" },
        body: new URLSearchParams({ csrf }),
      });
      if (!response.ok) {
        setState("error");
        return;
      }
      setState("idle");
      onSuccess();
    } catch {
      setState("error");
    }
  }

  return (
    <div className={styles.systemAppInstallAction}>
      <button type="button" disabled={!csrf || disabled || state === "installing"} onClick={install}>
        {state === "installing" ? t("Installing…") : t("Install")}
      </button>
      {state === "error" ? <small role="status">{t("CodeLocal.Cloud could not be reached.")}</small> : null}
    </div>
  );
}

function ProjectCard({ workspace, csrf, retry }: { workspace: WorkspaceItem; csrf?: string; retry: () => void }) {
  const { locale, t } = useTranslations();
  return (
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
              confirmMessage={t("Remove {name} from CodeLocal.Cloud? Your project and files will be preserved.", { name: workspace.workspaceName })}
              disabled={!workspace.runtimeOnline}
              onSuccess={retry}
            />
          </div>
        </details>
      </div>
    </article>
  );
}

function InstalledSystemAppCard({ workspace }: { workspace: WorkspaceItem }) {
  const { t } = useTranslations();
  return (
    <article className={`${styles.workspaceCard} ${styles.systemAppCard}`}>
      <Link
        className={styles.workspaceChatLink}
        href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}
        aria-label={t("Open Code Graph for {name}", { name: workspace.workspaceName })}
      >
        <span className={styles.workspaceFolderLarge} data-state={workspace.status} aria-hidden="true">
          <AppIcon name="runtime" size={25} />
          <i />
        </span>
        <span className={styles.workspaceCardBody}>
          <span className={styles.systemAppTitleLine}><h2>Video Studio</h2><b>CodeLocal.Cloud App</b></span>
          <span>{workspace.deviceName} · {t(statusLabel(workspace.status))}</span>
          <small>{t("Managed by CodeLocal.Cloud")}</small>
        </span>
        <span className={styles.workspaceChatAction}>{t("Open app")} <AppIcon name="chevron-right" size={14} /></span>
      </Link>
    </article>
  );
}

export function LiveWorkspaces() {
  const { t } = useTranslations();
  const { state, retry } = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const devices = useDashboardResource("/api/v1/devices", isDevicesResource);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const csrf = account.state.kind === "ready" ? account.state.value.csrf : undefined;
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);

  if (state.kind !== "ready") {
    return <DashboardResourceFeedback label={t("Projects")} {...(state.kind === "error" ? { kind: "error" as const, message: state.message, onRetry: retry } : { kind: state.kind })} />;
  }

  const resource = state.value;
  const needle = query.trim().toLowerCase();
  const userProjects = resource.items.filter((workspace) => !workspace.systemApp);
  const systemApps = resource.items.filter((workspace) => workspace.systemApp || workspace.workspaceId === videoStudioWorkspaceID);
  const filteredProjects = needle
    ? userProjects.filter((workspace) => `${workspace.workspaceName} ${workspace.deviceName}`.toLowerCase().includes(needle))
    : userProjects;
  const totalPages = Math.max(1, Math.ceil(filteredProjects.length / dashboardPageSize));
  const currentPage = Math.min(page, totalPages);
  const visibleProjects = filteredProjects.slice((currentPage - 1) * dashboardPageSize, currentPage * dashboardPageSize);
  const deviceItems = devices.state.kind === "ready" ? devices.state.value.items.filter((device) => device.status !== "revoked") : [];
  const appSlots = deviceItems
    .map((device) => ({ device, workspace: systemApps.find((workspace) => workspace.deviceId === device.deviceId && workspace.workspaceId === videoStudioWorkspaceID) }))
    .filter(({ device }) => !needle || `video studio codelocal app ${device.deviceName}`.toLowerCase().includes(needle));
  const orphanApps = systemApps.filter((workspace) => !deviceItems.some((device) => device.deviceId === workspace.deviceId));
  const onlineProjects = userProjects.filter((workspace) => workspace.status === "active").length;
  const inactiveProjects = userProjects.length - onlineProjects;

  return (
    <section className={styles.workspaceSection} aria-live="polite">
      <div className={styles.workspaceToolbar}>
        <div className={styles.summaryPills} aria-label={t("Project summary")}>
          <span>{t("{count} projects", { count: userProjects.length })}</span>
          <span data-state="active"><i />{t("{count} online", { count: onlineProjects })}</span>
          {inactiveProjects > 0 ? <span data-state="sleeping"><i />{t("{count} inactive", { count: inactiveProjects })}</span> : null}
        </div>
        <DashboardListControls
          query={query}
          onQueryChange={(value) => { setQuery(value); setPage(1); }}
          page={currentPage}
          totalPages={totalPages}
          totalResults={filteredProjects.length}
          onPageChange={setPage}
          placeholder={t("Search projects")}
        />
      </div>

      <div className={styles.workspaceGroup}>
        <div className={styles.workspaceGroupHead}>
          <div><h2>{t("Your Projects")}</h2><p>{t("Workspaces connected to your account")}</p></div>
          <span>{userProjects.length}</span>
        </div>
        {visibleProjects.length === 0 ? (
          <div className={styles.appleEmptyState}>
            <span className={styles.emptyFolderIcon} aria-hidden="true" />
            <strong>{t(query ? "No matching workspaces" : "No workspaces yet")}</strong>
            <p>{query ? t("Try another search.") : t("Run {command} in your project directory to connect your workspace.", { command: "codelocal ." })}</p>
          </div>
        ) : (
          <div className={styles.workspaceGrid}>
            {visibleProjects.map((workspace) => <ProjectCard key={`${workspace.deviceId}:${workspace.workspaceId}`} workspace={workspace} csrf={csrf} retry={retry} />)}
          </div>
        )}
      </div>

      <div className={styles.workspaceGroup}>
        <div className={styles.workspaceGroupHead}>
          <div><h2>{t("CodeLocal.Cloud Apps")}</h2><p>{t("Apps managed by CodeLocal.Cloud for AI-assisted work")}</p></div>
          <span>{systemApps.length}</span>
        </div>
        <div className={styles.workspaceGrid}>
          {appSlots.map(({ device, workspace }) => workspace ? (
            <InstalledSystemAppCard key={`${device.deviceId}:video-studio`} workspace={workspace} />
          ) : (
            <article className={`${styles.workspaceCard} ${styles.systemAppCard}`} key={`${device.deviceId}:video-studio`}>
              <div className={styles.workspaceChatLink}>
                <span className={styles.workspaceFolderLarge} data-state={device.status === "online" ? "active" : "offline"} aria-hidden="true">
                  <AppIcon name="runtime" size={25} />
                  <i />
                </span>
                <span className={styles.workspaceCardBody}>
                  <span className={styles.systemAppTitleLine}><h2>Video Studio</h2><b>CodeLocal.Cloud App</b></span>
                  <span>{device.deviceName} · {device.status === "online" ? t("Ready") : t("Offline")}</span>
                  <small>{t("Create, edit and render videos")}</small>
                </span>
                <InstallSystemAppButton
                  deviceId={device.deviceId}
                  csrf={csrf}
                  disabled={device.status !== "online"}
                  onSuccess={() => { retry(); devices.retry(); }}
                />
              </div>
            </article>
          ))}
          {orphanApps.map((workspace) => <InstalledSystemAppCard key={`${workspace.deviceId}:video-studio-orphan`} workspace={workspace} />)}
        </div>
      </div>
    </section>
  );
}
