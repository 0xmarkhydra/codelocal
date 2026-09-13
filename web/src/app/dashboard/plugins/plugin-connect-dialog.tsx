"use client";

import { useEffect, useRef, useState } from "react";
import { AppIcon } from "../app-icon";
import styles from "./plugins.module.css";
import { useTranslations } from "@/lib/i18n/provider";

type PluginConnection = {
  deviceId: string;
  connection: string;
  executionTarget: "local" | "cloud";
  workspaceKey: string;
  serverName: string;
  endpoint: string;
  credentialRef?: string;
  state: "configured" | "ready" | "error";
  toolCount: number;
  lastError?: string;
  connectedAt?: number;
  updatedAt: number;
};

type PluginItem = {
  id: string;
  name: string;
  version: string;
  description?: string;
  publisher: string;
  publisherVerified: boolean;
  categories?: string[];
  capabilities?: string[];
  featured?: boolean;
  installed: boolean;
  installationState?: string;
  installedAt?: number;
  updateAvailable?: boolean;
  setupRequired?: boolean;
  connections?: PluginConnection[];
  connectedCount?: number;
  system?: boolean;
  executionTargets?: Array<"local" | "cloud">;
  serverName?: string;
};

type WorkspaceItem = {
  deviceId: string;
  deviceName: string;
  workspaceId: string;
  workspaceName: string;
  runtimeOnline: boolean;
};

type ConnectionInput = {
  deviceId: string;
  workspaceId: string;
  endpoint: string;
  bearerEnv: string;
  bearerToken: string;
  executionTarget: "local" | "cloud";
};

const hostedPenpotMcpEndpoint = "https://design.codelocal.cloud/mcp/stream";

const permissionLabels: Record<string, string> = {
  external_read: "Read external data",
  external_write: "Create or update external data",
  external_delete: "Delete external data",
  network: "Connect to external services",
  project_read: "Read project files",
  project_write: "Edit project files",
  local_shell: "Run local commands",
  local_browser: "Use browser automation",
  local_credentials: "Use approved local credentials",
  computer_control: "Control approved desktop apps",
  background_task: "Run scheduled/background work",
};

function workspaceValue(workspace: WorkspaceItem) {
  return `${encodeURIComponent(workspace.deviceId)}|${encodeURIComponent(workspace.workspaceId)}`;
}

function parseWorkspaceValue(value: string) {
  const separator = value.indexOf("|");
  if (separator < 0) return null;
  try {
    return {
      deviceId: decodeURIComponent(value.slice(0, separator)),
      workspaceId: decodeURIComponent(value.slice(separator + 1)),
    };
  } catch {
    return null;
  }
}

function workspaceLabel(workspace: WorkspaceItem) {
  return `${workspace.deviceName} · ${workspace.workspaceName}`;
}

function connectionDeviceLabel(connection: PluginConnection, workspaces: WorkspaceItem[], cloudLabel: string) {
  if (connection.executionTarget === "cloud") return cloudLabel;
  const workspace = workspaces.find((item) => item.deviceId === connection.deviceId);
  return workspace?.deviceName || connection.deviceId;
}

export function PluginConnectDialog({
  plugin,
  busy,
  workspaces,
  install,
  connect,
  disconnect,
  uninstall,
  onClose,
}: {
  plugin: PluginItem;
  busy: boolean;
  workspaces: WorkspaceItem[];
  install: (plugin: PluginItem) => Promise<boolean>;
  connect: (plugin: PluginItem, input: ConnectionInput) => Promise<boolean>;
  disconnect: (plugin: PluginItem, connection: PluginConnection) => Promise<void>;
  uninstall: (plugin: PluginItem) => Promise<void>;
  onClose: () => void;
}) {
  const { t, message } = useTranslations();
  const closeRef = useRef<HTMLButtonElement>(null);
  const capabilities = plugin.capabilities ?? [];
  const connections = plugin.connections ?? [];
  const onlineWorkspaces = workspaces.filter((workspace) => workspace.runtimeOnline);
  const isPenpot = plugin.id === "penpot";
  const supportsCloud = (plugin.executionTargets ?? []).includes("cloud");
  const supportsDevice = (plugin.executionTargets ?? []).some((target) => target === "local") || (plugin.executionTargets ?? []).length === 0;
  const existing = connections[0];
  const [target, setTarget] = useState<"cloud" | "device">(() => {
    if (existing?.executionTarget === "cloud") return "cloud";
    return isPenpot && supportsCloud ? "cloud" : "device";
  });
  const [showPermissions, setShowPermissions] = useState(false);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [endpoint, setEndpoint] = useState(() => existing?.endpoint ?? (isPenpot ? hostedPenpotMcpEndpoint : ""));
  const [bearerToken, setBearerToken] = useState("");
  const [bearerEnv, setBearerEnv] = useState(() => existing?.credentialRef?.startsWith("CODELOCAL_PLUGIN_") ? "" : existing?.credentialRef ?? "");

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      onClose();
    };
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", closeOnEscape);
    window.requestAnimationFrame(() => closeRef.current?.focus());
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [onClose]);

  async function submitConnection() {
    if (target === "cloud") {
      if (!endpoint.trim() || !bearerToken.trim()) return;
      await connect(plugin, {
        deviceId: "",
        workspaceId: "",
        endpoint: endpoint.trim(),
        bearerEnv: "",
        bearerToken: bearerToken.trim(),
        executionTarget: "cloud",
      });
      setBearerToken("");
      return;
    }
    const selected = parseWorkspaceValue(selectedWorkspace);
    if (!selected || !endpoint.trim()) return;
    await connect(plugin, {
      ...selected,
      endpoint: endpoint.trim(),
      bearerEnv: bearerEnv.trim(),
      bearerToken: bearerToken.trim(),
      executionTarget: "local",
    });
    setBearerToken("");
  }

  const canSubmit = target === "cloud"
    ? Boolean(endpoint.trim()) && Boolean(bearerToken.trim())
    : Boolean(selectedWorkspace) && Boolean(endpoint.trim()) && !(isPenpot && connections.length === 0 && !bearerToken.trim());

  return (
    <div className={styles.dialogLayer}>
      <button aria-label={t("Close Plugin configuration")} className={styles.dialogBackdrop} onClick={onClose} type="button" />
      <section aria-modal="true" className={styles.dialog} role="dialog" aria-labelledby={`plugin-dialog-${plugin.id}`}>
        <header className={styles.dialogHeader}>
          <div className={styles.dialogTitle}>
            <h2 id={`plugin-dialog-${plugin.id}`}>{plugin.name}</h2>
            <p>{plugin.publisher} · v{plugin.version}</p>
          </div>
          <button
            aria-label={t("Close Plugin configuration")}
            className={styles.iconButton}
            disabled={busy}
            onClick={onClose}
            ref={closeRef}
            title={t("Close Plugin configuration")}
            type="button"
          >
            <AppIcon name="close" size={16} />
          </button>
        </header>

        {supportsCloud && supportsDevice && (
          <div className={styles.targetChoice} role="group" aria-label={t("Where should this Plugin run?")}>
            <button
              aria-pressed={target === "cloud"}
              className={styles.targetOption}
              data-selected={target === "cloud" || undefined}
              disabled={busy}
              onClick={() => setTarget("cloud")}
              type="button"
            >
              <strong>{t("Cloud")}</strong>
              <span>{t("Works without your device")}</span>
            </button>
            <button
              aria-pressed={target === "device"}
              className={styles.targetOption}
              data-selected={target === "device" || undefined}
              disabled={busy}
              onClick={() => setTarget("device")}
              type="button"
            >
              <strong>{t("Your device")}</strong>
              <span>{t("Connects from your device")}</span>
            </button>
          </div>
        )}

        {target === "cloud" ? (
          <div className={styles.configPanel}>
            <div className={styles.configHeader}>
              <div>
                <strong>{t("Configure connection")}</strong>
                <span>{t("CodeLocal Cloud connects to this MCP endpoint directly.")}</span>
              </div>
            </div>
            <label className={styles.field}>
              <span>{t("MCP endpoint")}</span>
              <input
                autoComplete="off"
                inputMode="url"
                onChange={(event) => setEndpoint(event.target.value)}
                placeholder="https://example.com/mcp"
                readOnly={isPenpot}
                type="url"
                value={endpoint}
              />
            </label>
            <label className={styles.field}>
              <span>{isPenpot ? t("Penpot MCP key or copied server URL") : t("Bearer token")}</span>
              <input
                autoCapitalize="none"
                autoComplete="new-password"
                onChange={(event) => setBearerToken(event.target.value)}
                placeholder={t("Stored encrypted by CodeLocal")}
                spellCheck={false}
                type="password"
                value={bearerToken}
              />
            </label>
            <p className={styles.credentialNote}>
              <AppIcon name="shield" size={13} />
              {t(isPenpot
                ? "Generate the key in Penpot under Account → Integrations → MCP Server. CodeLocal stores it encrypted and never writes it into your repository."
                : "Cloud connections run read-only tools only. Tools that change data need a device connection.")}
            </p>
          </div>
        ) : (
          <div className={styles.configPanel}>
            <div className={styles.configHeader}>
              <div>
                <strong>{t("Configure connection")}</strong>
                <span>{t(isPenpot
                  ? "Connect this device to CodeLocal's hosted Penpot MCP."
                  : "The MCP server is installed on your selected CodeLocal device.")}</span>
              </div>
            </div>
            <label className={styles.field}>
              <span>{t("Device / workspace")}</span>
              <select onChange={(event) => setSelectedWorkspace(event.target.value)} value={selectedWorkspace}>
                <option value="">{t("Select a running workspace")}</option>
                {onlineWorkspaces.map((workspace) => (
                  <option key={`${workspace.deviceId}-${workspace.workspaceId}`} value={workspaceValue(workspace)}>
                    {workspaceLabel(workspace)}
                  </option>
                ))}
              </select>
            </label>
            <label className={styles.field}>
              <span>{t("MCP endpoint")}</span>
              <input
                autoComplete="off"
                inputMode="url"
                onChange={(event) => setEndpoint(event.target.value)}
                placeholder="https://example.com/mcp"
                readOnly={isPenpot}
                type="url"
                value={endpoint}
              />
            </label>
            <label className={styles.field}>
              <span>{isPenpot ? t("Penpot MCP key or copied server URL") : <>{t("Bearer token")} <em>{t("Optional")}</em></>}</span>
              <input
                autoCapitalize="none"
                autoComplete="new-password"
                disabled={Boolean(bearerEnv)}
                onChange={(event) => setBearerToken(event.target.value)}
                placeholder={t("Stored encrypted by CodeLocal")}
                spellCheck={false}
                type="password"
                value={bearerToken}
              />
            </label>
            {!isPenpot && (
              <label className={styles.field}>
                <span>{t("Or local token environment variable")} <em>{t("Optional")}</em></span>
                <input
                  autoCapitalize="none"
                  autoComplete="off"
                  disabled={Boolean(bearerToken)}
                  onChange={(event) => setBearerEnv(event.target.value)}
                  placeholder="GITHUB_TOKEN"
                  spellCheck={false}
                  value={bearerEnv}
                />
              </label>
            )}
            <p className={styles.credentialNote}>
              <AppIcon name="shield" size={13} />
              {t(isPenpot
                ? "Generate the key in Penpot under Account → Integrations → MCP Server. CodeLocal stores it encrypted and never writes it into your repository."
                : "Token values are encrypted server-side and materialized only for the selected runtime connection. Environment references remain local to your device.")}
            </p>
            {onlineWorkspaces.length === 0 && (
              <p className={styles.configWarning}>{t("No running CodeLocal workspace found. Start {command} on a paired device first.", { command: "codelocal" })}</p>
            )}
          </div>
        )}

        <button
          aria-expanded={showPermissions}
          className={styles.permissionsSummary}
          onClick={() => setShowPermissions((value) => !value)}
          type="button"
        >
          {capabilities.length === 0
            ? t("No runtime permissions declared")
            : t("Requires {count} permissions", { count: capabilities.length })} · {t(showPermissions ? "Hide permissions" : "Show permissions")}
        </button>
        {showPermissions && (
          <div className={styles.permissionList}>
            {capabilities.map((capability) => (
              <span key={capability}>{permissionLabels[capability] ?? capability}</span>
            ))}
          </div>
        )}

        {connections.length > 0 && (
          <div className={styles.connectionList}>
            {connections.map((connection) => (
              <div className={styles.connectionRow} key={`${connection.connection}-${connection.serverName}`}>
                <div className={styles.connectionCopy}>
                  <strong>{connectionDeviceLabel(connection, workspaces, t("CodeLocal Cloud"))}</strong>
                  <span>
                    {connection.state === "ready"
                      ? t("{count} tools ready", { count: connection.toolCount })
                      : t(connection.state === "error" ? "Connection needs attention" : "Configured")}
                  </span>
                  {connection.lastError && <small title={connection.lastError}>{message(connection.lastError)}</small>}
                </div>
                <button className={styles.textButton} disabled={busy} onClick={() => void disconnect(plugin, connection)} type="button">
                  {t("Disconnect")}
                </button>
              </div>
            ))}
          </div>
        )}

        <footer className={styles.dialogFooter}>
          {plugin.installed && !plugin.system && (
            <button
              className={styles.secondaryButton}
              disabled={busy || connections.length > 0}
              onClick={() => void uninstall(plugin)}
              title={connections.length > 0 ? t("Disconnect all devices before removing this Plugin") : undefined}
              type="button"
            >
              {t("Remove")}
            </button>
          )}
          {plugin.installed && plugin.updateAvailable && (
            <button className={styles.secondaryButton} disabled={busy} onClick={() => void install(plugin)} type="button">
              {t(busy ? "Updating…" : "Update")}
            </button>
          )}
          <button className={styles.secondaryButton} disabled={busy} onClick={onClose} type="button">{t("Cancel")}</button>
          <button className={styles.primaryButton} disabled={busy || !canSubmit} onClick={() => void submitConnection()} type="button">
            {t(busy ? "Connecting…" : "Connect & test")}
          </button>
        </footer>
      </section>
    </div>
  );
}
