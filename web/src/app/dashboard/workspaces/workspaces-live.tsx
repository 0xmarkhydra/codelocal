"use client";

import { isAccountResource } from "@/lib/contracts/account";
import { isWorkspacesResource } from "@/lib/contracts/resources";
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
  return (
    <section className={styles.livePanel} aria-live="polite">
      <div className={styles.liveHead}>
        <div>
          <span className={styles.eyebrow}>Authorized workspaces</span>
          <h2>Project folders visible to CodeLocal.</h2>
          <p>This list comes from the Go workspace catalog and omits local paths, capabilities and routing internals.</p>
        </div>
        <span className={styles.liveBadge}>LIVE BACKEND DATA</span>
      </div>

      <div className={styles.metricGrid}>
        <article className={styles.metricCard}><span>Authorized</span><strong>{resource.summary.total}</strong><p>Folders explicitly granted to CodeLocal.</p></article>
        <article className={styles.metricCard}><span>Active</span><strong>{resource.summary.active}</strong><p>Loaded for an active MCP session.</p></article>
        <article className={styles.metricCard}><span>Sleeping / offline</span><strong>{resource.summary.sleeping}<small> / {resource.summary.offline}</small></strong><p>Authorized without an active heavy runtime.</p></article>
      </div>

      <div className={styles.resourceList}>
        {resource.items.length === 0 ? (
          <p className={styles.emptyCopy}>No authorized workspace has synced yet. Run <code>codelocal .</code> inside a project to authorize it.</p>
        ) : resource.items.map((workspace) => (
          <article className={styles.resourceRow} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
            <span className={styles.stateDot} data-state={workspace.status} />
            <div className={styles.resourceIdentity}>
              <strong>{workspace.workspaceName}</strong>
              <span>{workspace.deviceName} · last seen {formatDashboardTime(workspace.lastSeenAt)}</span>
            </div>
            <div className={styles.resourceActions}>
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
