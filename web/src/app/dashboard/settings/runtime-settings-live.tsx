"use client";

import { type FormEvent, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isDevicesResource, isWorkspacesResource } from "@/lib/contracts/resources";
import { isRuntimeSettingsResource, type RuntimeScope } from "@/lib/contracts/runtime-settings";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { AppIcon } from "../app-icon";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./runtime-settings.module.css";

const scopes: Array<{ value: RuntimeScope; label: MessageKey }> = [
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
  const { locale, t } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  const [scope, setScope] = useState<RuntimeScope>("global");
  const [deviceId, setDeviceId] = useState("");
  const [workspaceId, setWorkspaceId] = useState("");
  const [configKey, setConfigKey] = useState("");
  const [configValue, setConfigValue] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [secretValue, setSecretValue] = useState("");
  const [showConfigForm, setShowConfigForm] = useState(false);
  const [showSecretForm, setShowSecretForm] = useState(false);
  const [editingConfigKey, setEditingConfigKey] = useState("");
  const [editingConfigValue, setEditingConfigValue] = useState("");
  const [editingSecretKey, setEditingSecretKey] = useState("");
  const [editingSecretValue, setEditingSecretValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: "success" | "error"; status?: number } | null>(null);

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

  function resetEditors() {
    setShowConfigForm(false);
    setShowSecretForm(false);
    setEditingConfigKey("");
    setEditingConfigValue("");
    setEditingSecretKey("");
    setEditingSecretValue("");
    setFeedback(null);
  }

  function selectScope(nextScope: RuntimeScope) {
    setScope(nextScope);
    resetEditors();
  }

  async function mutate(endpoint: string, values: Record<string, string>) {
    if (!csrf || !targetReady) return false;
    setSaving(true);
    setFeedback(null);
    try {
      const body = new URLSearchParams({ csrf, scope, deviceId: effectiveDeviceId, workspaceId: effectiveWorkspaceId, ...values });
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" },
        body,
      });
      if (!response.ok) {
        setFeedback({ kind: "error", status: response.status });
        return false;
      }
      setFeedback({ kind: "success" });
      settings.retry();
      return true;
    } catch {
      setFeedback({ kind: "error" });
      return false;
    } finally {
      setSaving(false);
    }
  }

  async function addConfig(event: FormEvent) {
    event.preventDefault();
    if (!configKey.trim()) return;
    if (await mutate("/api/v1/runtime/settings/config", { key: configKey.trim(), value: configValue, action: "set" })) {
      setConfigKey("");
      setConfigValue("");
      setShowConfigForm(false);
    }
  }

  async function addSecret(event: FormEvent) {
    event.preventDefault();
    if (!secretKey.trim() || !secretValue) return;
    if (await mutate("/api/v1/runtime/settings/secret", { name: secretKey.trim(), value: secretValue, action: "set" })) {
      setSecretKey("");
      setSecretValue("");
      setShowSecretForm(false);
    }
  }

  async function saveConfigEdit(key: string) {
    if (await mutate("/api/v1/runtime/settings/config", { key, value: editingConfigValue, action: "set" })) {
      setEditingConfigKey("");
      setEditingConfigValue("");
    }
  }

  async function saveSecretEdit(key: string) {
    if (!editingSecretValue) return;
    if (await mutate("/api/v1/runtime/settings/secret", { name: key, value: editingSecretValue, action: "set" })) {
      setEditingSecretKey("");
      setEditingSecretValue("");
    }
  }

  return (
    <div className={styles.shell}>
      <section className={styles.scopePanel} aria-label={t("Settings scope")}>
        <div className={styles.scopeIntro}>
          <span>{t("Scope")}</span>
          <strong>{t(scope === "global" ? "All runtimes" : scope === "device" ? "One device" : "One workspace")}</strong>
        </div>
        <div className={styles.scopeTabs}>
          {scopes.map((item) => (
            <button key={item.value} type="button" disabled={saving} data-active={scope === item.value} aria-pressed={scope === item.value} onClick={() => selectScope(item.value)}>
              {t(item.label)}
            </button>
          ))}
        </div>
        <div className={styles.targetSelectors}>
          {scope !== "global" && (
            <label>
              <span>{t("Device")}</span>
              <select disabled={saving} aria-label={t("Device")} value={effectiveDeviceId} onChange={(event) => { setDeviceId(event.target.value); setWorkspaceId(""); resetEditors(); }}>
                <option value="">{t("Select device")}</option>
                {deviceItems.map((item) => <option key={item.deviceId} value={item.deviceId}>{item.deviceName}</option>)}
              </select>
            </label>
          )}
          {scope === "workspace" && (
            <label>
              <span>{t("Workspace")}</span>
              <select disabled={saving} aria-label={t("Workspace")} value={effectiveWorkspaceId} onChange={(event) => { setWorkspaceId(event.target.value); resetEditors(); }}>
                <option value="">{t("Select workspace")}</option>
                {workspaceItems.map((item) => <option key={`${item.deviceId}:${item.workspaceId}`} value={item.workspaceId}>{item.workspaceName}</option>)}
              </select>
            </label>
          )}
        </div>
        {feedback && <span className={styles.feedback} role="status" data-kind={feedback.kind}>{feedback.kind === "success" ? t("Saved") : feedback.status ? t("CodeLocal.Cloud rejected this change ({status}).", { status: String(feedback.status) }) : t("CodeLocal.Cloud could not be reached.")}</span>}
      </section>

      {!targetReady ? (
        <div className={styles.emptyState}>
          <AppIcon name="target" size={19} />
          <strong>{t("Choose a target")}</strong>
          <span>{t("Select a device or workspace to edit this scope.")}</span>
        </div>
      ) : settings.state.kind !== "ready" ? (
        <div className={styles.resourceState}>
          <DashboardResourceFeedback label="Runtime settings" {...(settings.state.kind === "error" ? { kind: "error" as const, message: settings.state.message, onRetry: settings.retry } : { kind: settings.state.kind })} />
        </div>
      ) : (
        <>
          <div className={styles.settingsGrid}>
            <section className={styles.panel}>
              <header className={styles.panelHeader}>
                <div className={styles.panelTitle}>
                  <span className={styles.panelIcon}><AppIcon name="runtime" size={17} /></span>
                  <div>
                    <div className={styles.titleLine}><h2>{t("Configuration")}</h2><span>{number.format(Object.keys(settings.state.value.layer.values ?? {}).length)}</span></div>
                    <p>{t("Runtime values")}</p>
                  </div>
                </div>
                <button className={styles.iconButton} data-active={showConfigForm} type="button" disabled={saving} onClick={() => setShowConfigForm((current) => !current)} aria-expanded={showConfigForm} aria-label={t(showConfigForm ? "Cancel" : "Add config")} title={t(showConfigForm ? "Cancel" : "Add config")}>
                  <AppIcon name={showConfigForm ? "close" : "plus"} size={16} />
                </button>
              </header>

              <div className={styles.rows}>
                {Object.entries(settings.state.value.layer.values ?? {}).map(([key, value]) => (
                  <div className={styles.settingRow} key={key}>
                    <div className={styles.settingMain}>
                      <strong>{key}</strong>
                      {editingConfigKey === key ? (
                        <input value={editingConfigValue} onChange={(event) => setEditingConfigValue(event.target.value)} aria-label={t("New value for {key}", { key })} autoFocus />
                      ) : (
                        <small title={value}>{value || t("Empty")}</small>
                      )}
                    </div>
                    <div className={styles.rowActions}>
                      {editingConfigKey === key ? (
                        <>
                          <button className={`${styles.iconButton} ${styles.primaryAction}`} type="button" disabled={saving} onClick={() => void saveConfigEdit(key)} aria-label={t("Save {key}", { key })} title={t("Save")}>
                            <AppIcon name="check" size={15} />
                          </button>
                          <button className={styles.iconButton} type="button" disabled={saving} onClick={() => { setEditingConfigKey(""); setEditingConfigValue(""); }} aria-label={t("Cancel editing {key}", { key })} title={t("Cancel")}>
                            <AppIcon name="close" size={15} />
                          </button>
                        </>
                      ) : (
                        <>
                          <button className={styles.iconButton} type="button" disabled={saving} onClick={() => { setEditingConfigKey(key); setEditingConfigValue(value); }} aria-label={t("Edit {key}", { key })} title={t("Edit")}>
                            <AppIcon name="edit" size={15} />
                          </button>
                          <button className={`${styles.iconButton} ${styles.dangerAction}`} type="button" disabled={saving} onClick={() => void mutate("/api/v1/runtime/settings/config", { key, action: "delete" })} aria-label={t("Remove {key}", { key })} title={t("Remove")}>
                            <AppIcon name="trash" size={15} />
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
                {Object.keys(settings.state.value.layer.values ?? {}).length === 0 && (
                  <div className={styles.panelEmpty}><AppIcon name="plus" size={15} /><span>{t("No overrides in this scope")}</span></div>
                )}
              </div>

              {showConfigForm && (
                <form className={styles.addForm} onSubmit={addConfig}>
                  <input value={configKey} onChange={(event) => setConfigKey(event.target.value)} placeholder="FFMPEG_PATH" aria-label={t("Config key")} autoFocus />
                  <input value={configValue} onChange={(event) => setConfigValue(event.target.value)} placeholder={t("Value")} aria-label={t("Config value")} />
                  <button className={`${styles.iconButton} ${styles.primaryAction}`} disabled={saving || !csrf || !configKey.trim()} aria-label={t("Add config")} title={t("Add")}>
                    <AppIcon name="plus" size={16} />
                  </button>
                </form>
              )}
            </section>

            <section className={styles.panel}>
              <header className={styles.panelHeader}>
                <div className={styles.panelTitle}>
                  <span className={styles.panelIcon}><AppIcon name="shield" size={17} /></span>
                  <div>
                    <div className={styles.titleLine}><h2>{t("Secrets")}</h2><span>{number.format(Object.keys(settings.state.value.layer.secrets ?? {}).length)}</span></div>
                    <p>{t("Encrypted · memory-only")}</p>
                  </div>
                </div>
                <button className={styles.iconButton} data-active={showSecretForm} type="button" disabled={saving} onClick={() => setShowSecretForm((current) => !current)} aria-expanded={showSecretForm} aria-label={t(showSecretForm ? "Cancel" : "Add secret")} title={t(showSecretForm ? "Cancel" : "Add secret")}>
                  <AppIcon name={showSecretForm ? "close" : "plus"} size={16} />
                </button>
              </header>

              <div className={styles.rows}>
                {Object.entries(settings.state.value.layer.secrets ?? {}).map(([key]) => (
                  <div className={styles.settingRow} key={key}>
                    <div className={styles.settingMain}>
                      <strong>{key}</strong>
                      {editingSecretKey === key ? (
                        <input type="password" value={editingSecretValue} onChange={(event) => setEditingSecretValue(event.target.value)} placeholder={t("New secret value")} aria-label={t("New secret for {key}", { key })} autoComplete="new-password" autoFocus />
                      ) : (
                        <small className={styles.secretValue}>•••••••• <span>{t("Encrypted")}</span></small>
                      )}
                    </div>
                    <div className={styles.rowActions}>
                      {editingSecretKey === key ? (
                        <>
                          <button className={`${styles.iconButton} ${styles.primaryAction}`} type="button" disabled={saving || !editingSecretValue} onClick={() => void saveSecretEdit(key)} aria-label={t("Update {key}", { key })} title={t("Update")}>
                            <AppIcon name="check" size={15} />
                          </button>
                          <button className={styles.iconButton} type="button" disabled={saving} onClick={() => { setEditingSecretKey(""); setEditingSecretValue(""); }} aria-label={t("Cancel updating {key}", { key })} title={t("Cancel")}>
                            <AppIcon name="close" size={15} />
                          </button>
                        </>
                      ) : (
                        <>
                          <button className={styles.iconButton} type="button" disabled={saving} onClick={() => { setEditingSecretKey(key); setEditingSecretValue(""); }} aria-label={t("Update {key}", { key })} title={t("Update")}>
                            <AppIcon name="edit" size={15} />
                          </button>
                          <button className={`${styles.iconButton} ${styles.dangerAction}`} type="button" disabled={saving} onClick={() => void mutate("/api/v1/runtime/settings/secret", { name: key, action: "delete" })} aria-label={t("Remove {key}", { key })} title={t("Remove")}>
                            <AppIcon name="trash" size={15} />
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
                {Object.keys(settings.state.value.layer.secrets ?? {}).length === 0 && (
                  <div className={styles.panelEmpty}><AppIcon name="shield" size={15} /><span>{t("No secrets in this scope")}</span></div>
                )}
              </div>

              {showSecretForm && (
                <form className={styles.addForm} onSubmit={addSecret}>
                  <input value={secretKey} onChange={(event) => setSecretKey(event.target.value)} placeholder="VBEE_API_KEY" aria-label={t("Secret key")} autoComplete="off" autoFocus />
                  <input type="password" value={secretValue} onChange={(event) => setSecretValue(event.target.value)} placeholder={t("Secret value")} aria-label={t("Secret value")} autoComplete="new-password" />
                  <button className={`${styles.iconButton} ${styles.primaryAction}`} disabled={saving || !csrf || !secretKey.trim() || !secretValue} aria-label={t("Add secret")} title={t("Add")}>
                    <AppIcon name="plus" size={16} />
                  </button>
                </form>
              )}
            </section>
          </div>

          <section className={styles.capabilitiesPanel}>
            <header className={styles.capabilitiesHeader}>
              <div className={styles.panelTitle}>
                <span className={styles.panelIcon}><AppIcon name="skill" size={17} /></span>
                <div><h2>{t("Capabilities")}</h2><p>{t("System projects available to this runtime")}</p></div>
              </div>
              <span className={styles.capabilityCount}>{number.format((settings.state.value.effective.systemProjects ?? []).length)}</span>
            </header>
            <div className={styles.capabilityGrid}>
              {(settings.state.value.effective.systemProjects ?? []).map((project) => (
                <div className={styles.capability} key={project.id}>
                  <span className={styles.capabilityIcon}><AppIcon name="runtime" size={18} /></span>
                  <span className={styles.capabilityCopy}>
                    <strong>{project.id === "openmontage" ? "Video Studio" : project.name}</strong>
                    <small>{project.id === "openmontage" ? "OpenMontage" : t(project.managed ? "Managed by CodeLocal.Cloud" : "Workspace managed")}</small>
                  </span>
                  <span className={styles.statusPill} data-enabled={project.enabled}>{t(project.enabled ? "Enabled" : "Disabled")}</span>
                </div>
              ))}
              {(settings.state.value.effective.systemProjects ?? []).length === 0 && (
                <div className={styles.panelEmpty}><AppIcon name="skill" size={15} /><span>{t("No system capabilities")}</span></div>
              )}
            </div>
          </section>
        </>
      )}
    </div>
  );
}
