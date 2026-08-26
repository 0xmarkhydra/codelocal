"use client";

import { FormEvent, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource, isWorkspacesResource } from "@/lib/contracts/resources";
import { isRuntimeSettingsResource, type RuntimeScope } from "@/lib/contracts/runtime-settings";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { AppIcon } from "../app-icon";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./runtime-settings.module.css";

const scopes: Array<{ value: RuntimeScope; label: string }> = [
  { value: "global", label: "Global" },
  { value: "device", label: "Device" },
  { value: "workspace", label: "Workspace" },
];

function query(scope: RuntimeScope, deviceId: string, workspaceId: string) {
  const params = new URLSearchParams({ scope });
  if (scope !== "global" && deviceId) params.set("deviceId", deviceId);
  if (scope === "workspace" && workspaceId) params.set("workspaceId", workspaceId);
  return `/api/v1/runtime/settings?${params.toString()}`;
}

export function RuntimeSettingsLive() {
  const [scope, setScope] = useState<RuntimeScope>("global");
  const [deviceId, setDeviceId] = useState("");
  const [workspaceId, setWorkspaceId] = useState("");
  const [configKey, setConfigKey] = useState("");
  const [configValue, setConfigValue] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [secretValue, setSecretValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");

  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const devices = useDashboardResource("/api/v1/devices", isDevicesResource);
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const deviceItems = devices.state.kind === "ready" ? devices.state.value.items : [];
  const effectiveDeviceId = deviceId || deviceItems[0]?.deviceId || "";
  const workspaceItems = workspaces.state.kind === "ready"
    ? workspaces.state.value.items.filter((item) => !effectiveDeviceId || item.deviceId === effectiveDeviceId)
    : [];
  const effectiveWorkspaceId = workspaceItems.some((item) => item.workspaceId === workspaceId)
    ? workspaceId
    : workspaceItems[0]?.workspaceId ?? "";
  const settingsURL = query(scope, effectiveDeviceId, effectiveWorkspaceId);
  const settings = useDashboardResource(settingsURL, isRuntimeSettingsResource);

  const csrf = account.state.kind === "ready" ? account.state.value.csrf : "";
  const targetReady = scope === "global" || (scope === "device" ? Boolean(effectiveDeviceId) : Boolean(effectiveDeviceId && effectiveWorkspaceId));

  async function mutate(endpoint: string, values: Record<string, string>) {
    if (!csrf || !targetReady) return;
    setSaving(true);
    setMessage("");
    try {
      const body = new URLSearchParams({ csrf, scope, deviceId: effectiveDeviceId, workspaceId: effectiveWorkspaceId, ...values });
      const response = await fetch(endpoint, { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" }, body });
      if (!response.ok) throw new Error(`CodeLocal rejected this change (${response.status}).`);
      setMessage("Updated");
      settings.retry();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Update failed.");
    } finally {
      setSaving(false);
    }
  }

  async function addConfig(event: FormEvent) {
    event.preventDefault();
    if (!configKey.trim()) return;
    await mutate("/api/v1/runtime/settings/config", { key: configKey.trim(), value: configValue, action: "set" });
    setConfigKey("");
    setConfigValue("");
  }

  async function addSecret(event: FormEvent) {
    event.preventDefault();
    if (!secretKey.trim() || !secretValue) return;
    await mutate("/api/v1/runtime/settings/secret", { name: secretKey.trim(), value: secretValue, action: "set" });
    setSecretKey("");
    setSecretValue("");
  }

  return (
    <div className={styles.shell}>
      <div className={styles.scopeBar}>
        <div className={styles.scopeTabs}>
          {scopes.map((item) => (
            <button key={item.value} type="button" data-active={scope === item.value} onClick={() => setScope(item.value)}>{item.label}</button>
          ))}
        </div>
        {scope !== "global" && (
          <select aria-label="Device" value={effectiveDeviceId} onChange={(event) => { setDeviceId(event.target.value); setWorkspaceId(""); }}>
            <option value="">Select device</option>
            {deviceItems.map((item) => <option key={item.deviceId} value={item.deviceId}>{item.deviceName}</option>)}
          </select>
        )}
        {scope === "workspace" && (
          <select aria-label="Workspace" value={effectiveWorkspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
            <option value="">Select workspace</option>
            {workspaceItems.map((item) => <option key={`${item.deviceId}:${item.workspaceId}`} value={item.workspaceId}>{item.workspaceName}</option>)}
          </select>
        )}
        {message && <span className={styles.message}>{message}</span>}
      </div>

      {!targetReady ? (
        <div className={styles.empty}>Chọn target để cấu hình.</div>
      ) : settings.state.kind !== "ready" ? (
        <DashboardResourceFeedback label="Runtime settings" {...(settings.state.kind === "error" ? { kind: "error" as const, message: settings.state.message, onRetry: settings.retry } : { kind: settings.state.kind })} />
      ) : (
        <div className={styles.grid}>
          <section className={styles.card}>
            <header><span className={styles.icon}><AppIcon name="runtime" size={18} /></span><div><h2>Config</h2><p>Environment/runtime values.</p></div></header>
            <div className={styles.rows}>
              {Object.entries(settings.state.value.layer.values ?? {}).map(([key, value]) => (
                <div className={styles.row} key={key}><span><strong>{key}</strong><small>{value || "Empty"}</small></span><button type="button" disabled={saving} onClick={() => void mutate("/api/v1/runtime/settings/config", { key, action: "delete" })}>Remove</button></div>
              ))}
              {Object.keys(settings.state.value.layer.values ?? {}).length === 0 && <div className={styles.muted}>No overrides in this scope.</div>}
            </div>
            <form className={styles.form} onSubmit={addConfig}>
              <input value={configKey} onChange={(event) => setConfigKey(event.target.value)} placeholder="FFMPEG_PATH" aria-label="Config key" />
              <input value={configValue} onChange={(event) => setConfigValue(event.target.value)} placeholder="Value" aria-label="Config value" />
              <button disabled={saving || !csrf}>Add</button>
            </form>
          </section>

          <section className={styles.card}>
            <header><span className={styles.icon}><AppIcon name="shield" size={18} /></span><div><h2>Secrets</h2><p>Encrypted in Cloud, memory-only in Runtime.</p></div></header>
            <div className={styles.rows}>
              {Object.entries(settings.state.value.layer.secrets ?? {}).map(([key]) => (
                <div className={styles.row} key={key}><span><strong>{key}</strong><small>••••••••</small></span><button type="button" disabled={saving} onClick={() => void mutate("/api/v1/runtime/settings/secret", { name: key, action: "delete" })}>Remove</button></div>
              ))}
              {Object.keys(settings.state.value.layer.secrets ?? {}).length === 0 && <div className={styles.muted}>No secrets in this scope.</div>}
            </div>
            <form className={styles.form} onSubmit={addSecret}>
              <input value={secretKey} onChange={(event) => setSecretKey(event.target.value)} placeholder="VBEE_API_KEY" aria-label="Secret key" autoComplete="off" />
              <input type="password" value={secretValue} onChange={(event) => setSecretValue(event.target.value)} placeholder="Secret value" aria-label="Secret value" autoComplete="new-password" />
              <button disabled={saving || !csrf}>Save</button>
            </form>
          </section>

          <section className={`${styles.card} ${styles.capabilityCard}`}>
            <header><span className={styles.icon}><AppIcon name="skill" size={18} /></span><div><h2>Capabilities</h2><p>System projects available to every workspace.</p></div></header>
            <div className={styles.rows}>
              {(settings.state.value.effective.systemProjects ?? []).map((project) => (
                <div className={styles.capability} key={project.id}>
                  <span className={styles.capabilityIcon}>🎬</span>
                  <span><strong>{project.id === "openmontage" ? "Video Studio" : project.name}</strong><small>{project.managed ? "Managed by CodeLocal" : "Workspace managed"}</small></span>
                  <i data-enabled={project.enabled} />
                </div>
              ))}
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
