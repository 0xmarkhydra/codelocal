"use client";

import Link from "next/link";
import { DashboardOverview, isDashboardOverview } from "@/lib/contracts/dashboard";
import type { AppIconName } from "./app-icon";
import { AppIcon } from "./app-icon";
import { DashboardResourceFeedback } from "./dashboard-resource-feedback";
import styles from "./dashboard.module.css";
import overviewStyles from "./overview.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

const compactNumber = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });

function workspaceStateLabel(status: DashboardOverview["workspaces"]["recent"][number]["status"]) {
  switch (status) {
    case "active": return "Online";
    case "sleeping": return "Sleeping";
    default: return "Offline";
  }
}

export function LiveOverview() {
  const { state, retry } = useDashboardResource("/api/v1/dashboard/overview", isDashboardOverview);

  if (state.kind !== "ready") {
    return <DashboardResourceFeedback label="Connections" {...(state.kind === "error" ? { kind: "error" as const, message: state.message, onRetry: retry } : { kind: state.kind })} />;
  }

  const overview = state.value;
  const runtimeOnline = overview.devices.online > 0;
  const workspaceOnline = overview.workspaces.active > 0;

  return (
    <div className={overviewStyles.overviewShell}>
      <section className={overviewStyles.connectionCards} aria-label="Connection status">
        <ConnectionCard icon="user" kind="account" label="Account" value="Signed in" state="online" />
        <ConnectionCard icon="runtime" kind="runtime" label="Local Runtime" value={runtimeOnline ? `${overview.devices.online} online` : "Offline"} state={runtimeOnline ? "online" : "offline"} />
        <ConnectionCard icon="folder" kind="workspace" label="Workspaces" value={`${overview.workspaces.active}/${overview.workspaces.total} active`} state={workspaceOnline ? "online" : "idle"} />
        <ConnectionCard icon="connection" kind="mcp" label="MCP activity" value={overview.usage.available ? `${compactNumber.format(overview.usage.last24h.calls)} calls · 24h` : "No telemetry"} state={overview.usage.available ? "online" : "idle"} />
      </section>

      <section className={overviewStyles.connectionMap}>
        <div className={overviewStyles.panelHead}>
          <div><span className={styles.eyebrow}>Connection map</span><strong>Luồng hoạt động hiện tại</strong></div>
          <Link href="/dashboard/workspaces">Workspaces <AppIcon name="chevron-right" size={13} /></Link>
        </div>
        <div className={overviewStyles.connectionFlow} aria-label="CodeLocal connection flow">
          <FlowNode icon="user" label="Account" state="online" />
          <FlowArrow active />
          <FlowNode icon="runtime" label="Runtime" state={runtimeOnline ? "online" : "offline"} />
          <FlowArrow active={runtimeOnline} />
          <FlowNode icon="folder" label="Workspace" state={workspaceOnline ? "online" : runtimeOnline ? "idle" : "offline"} />
          <FlowArrow active={workspaceOnline} />
          <FlowNode icon="brain" label="Brain" state={overview.workspaces.total > 0 ? "online" : "idle"} />
          <FlowArrow active={overview.workspaces.total > 0} />
          <FlowNode icon="chat" label="Thánh Gióng" state="online" />
        </div>
      </section>

      <section className={overviewStyles.recentPanel}>
        <div className={overviewStyles.panelHead}>
          <div><span className={styles.eyebrow}>Recent</span><strong>Workspaces gần đây</strong></div>
          <span>{overview.devices.online}/{overview.devices.paired} máy online</span>
        </div>
        {overview.workspaces.recent.length === 0 ? <p className={overviewStyles.empty}>Chưa có workspace.</p> : (
          <div className={overviewStyles.workspaceList}>
            {overview.workspaces.recent.slice(0, 6).map((workspace) => (
              <div className={overviewStyles.workspaceRow} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
                <span className={overviewStyles.workspaceFolder} data-state={workspace.status} aria-hidden="true"><AppIcon name="folder" size={17} /></span>
                <div><strong>{workspace.workspaceName}</strong><small>{workspace.deviceName}</small></div>
                <span className={overviewStyles.workspaceState} data-state={workspace.status}><i />{workspaceStateLabel(workspace.status)}</span>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function ConnectionCard({ icon, kind, label, value, state }: { icon: AppIconName; kind: string; label: string; value: string; state: "online" | "idle" | "offline" }) {
  return (
    <article>
      <span className={overviewStyles.connectionIcon} data-kind={kind}><AppIcon name={icon} size={18} /></span>
      <div><small>{label}</small><strong>{value}</strong></div>
      <i data-state={state} />
    </article>
  );
}

function FlowNode({ icon, label, state }: { icon: AppIconName; label: string; state: "online" | "idle" | "offline" }) {
  return <div className={overviewStyles.flowNode} data-state={state}><span><AppIcon name={icon} size={17} /></span><strong>{label}</strong><i /></div>;
}

function FlowArrow({ active }: { active: boolean }) {
  return <span className={overviewStyles.flowArrow} data-active={active || undefined} aria-hidden="true"><i /></span>;
}
