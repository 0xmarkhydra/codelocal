"use client";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey, MessageValues } from "@/lib/i18n/messages";

import { useCallback, useEffect, useMemo, useState } from "react";
import { isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { AppIcon } from "../app-icon";
import styles from "./plugins.module.css";

type PluginView = "explore" | "installed";
type WorkspaceItem = WorkspacesResource["items"][number];

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

type PluginsResource = {
  items: PluginItem[];
  installedCount: number;
};

type AccountResource = {
  csrf: string;
  requiresReauthentication: boolean;
};

type ConnectionInput = {
  deviceId: string;
  workspaceId: string;
  endpoint: string;
  bearerEnv: string;
  bearerToken: string;
};

type Notice = { kind: "success" | "error"; text: string; values?: MessageValues } | null;

const hostedPenpotMcpEndpoint = "https://design.codelocal.cloud/mcp/stream";

const permissionLabels: Record<string, MessageKey> = {
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

function responseError(response: Response) {
  return response.json()
    .then((payload: { detail?: string; error?: string }) => {
      if (payload.detail) return payload.detail;
      switch (payload.error) {
        case "invalid_csrf": return "Security token expired. Reload the page and try again.";
        case "reauthentication_required": return "Sign in again before changing Plugin access.";
        case "plugin_not_found": return "This Plugin is no longer available.";
        case "plugin_not_installed": return "Install this Plugin before configuring a connection.";
        case "plugin_install_failed": return "CodeLocal could not save the Plugin installation.";
        case "plugin_uninstall_failed": return "CodeLocal could not remove the Plugin installation.";
        case "plugin_disconnect_required": return "Disconnect this Plugin from every device before removing it.";
        case "system_plugin_required": return "System Plugins are managed by CodeLocal and cannot be removed.";
        case "plugin_credential_store_failed": return "CodeLocal could not store this credential securely.";
        case "plugin_credential_materialization_failed": return "CodeLocal could not materialize the encrypted credential for this device.";
        case "plugin_connection_not_found": return "This Plugin connection no longer exists.";
        case "plugin_workspace_unavailable": return "That CodeLocal workspace is offline or unavailable.";
        case "client_upgrade_required": return "Update the CodeLocal client on that device before configuring Plugins.";
        case "plugin_connect_failed": return "The local CodeLocal runtime could not connect this Plugin.";
        case "plugin_disconnect_failed": return "The local CodeLocal runtime could not disconnect this Plugin.";
        default: return `Request failed (${response.status}).`;
      }
    })
    .catch(() => `Request failed (${response.status}).`);
}

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

function connectionDeviceLabel(connection: PluginConnection, workspaces: WorkspaceItem[]) {
  const workspace = workspaces.find((item) => item.deviceId === connection.deviceId);
  return workspace?.deviceName || connection.deviceId;
}

function PluginCard({
  plugin,
  busy,
  workspaces,
  install,
  uninstall,
  connect,
  disconnect,
}: {
  plugin: PluginItem;
  busy: boolean;
  workspaces: WorkspaceItem[];
  install: (plugin: PluginItem) => Promise<void>;
  uninstall: (plugin: PluginItem) => Promise<void>;
  connect: (plugin: PluginItem, input: ConnectionInput) => Promise<boolean>;
  disconnect: (plugin: PluginItem, connection: PluginConnection) => Promise<void>;
}) {
  const { t, message } = useTranslations();
  const capabilities = plugin.capabilities ?? [];
  const categories = (plugin.categories ?? []).slice(0, 3);
  const connections = plugin.connections ?? [];
  const onlineWorkspaces = workspaces.filter((workspace) => workspace.runtimeOnline);
  const isPenpot = plugin.id === "penpot";
  const monogram = plugin.name.slice(0, 2).toUpperCase();
  const readyConnections = plugin.connectedCount ?? connections.filter((connection) => connection.state === "ready").length;
  const hasConnectionError = connections.some((connection) => connection.state === "error");
  const [showConfig, setShowConfig] = useState(false);
  const [selectedWorkspace, setSelectedWorkspace] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [bearerEnv, setBearerEnv] = useState("");
  const [bearerToken, setBearerToken] = useState("");

  function openConfigure() {
    const existing = connections[0];
    const preferred = existing
      ? onlineWorkspaces.find((workspace) => workspace.deviceId === existing.deviceId)
      : onlineWorkspaces[0];
    setSelectedWorkspace(preferred ? workspaceValue(preferred) : "");
    setEndpoint(existing?.endpoint ?? (isPenpot ? hostedPenpotMcpEndpoint : ""));
    setBearerEnv(existing?.credentialRef?.startsWith("CODELOCAL_PLUGIN_") ? "" : existing?.credentialRef ?? "");
    setBearerToken("");
    setShowConfig(true);
  }

  async function submitConnection() {
    const selected = parseWorkspaceValue(selectedWorkspace);
    if (!selected || !endpoint.trim() || (isPenpot && connections.length === 0 && !bearerToken.trim())) return;
    const connected = await connect(plugin, {
      ...selected,
      endpoint: endpoint.trim(),
      bearerEnv: bearerEnv.trim(),
      bearerToken: bearerToken.trim(),
    });
    setBearerToken("");
    if (connected) setShowConfig(false);
  }

  return (
    <article className={styles.pluginCard} data-installed={plugin.installed || undefined}>
      <div className={styles.cardTop}>
        <div className={styles.pluginIdentity}>
          <div className={styles.pluginIcon} aria-hidden="true">{monogram}</div>
          <div>
            <div className={styles.nameRow}>
              <h2>{plugin.name}</h2>
              {plugin.publisherVerified && <span className={styles.verified} title={t("Verified publisher")}><AppIcon name="check" size={11} /> {t("Verified")}</span>}
            </div>
            <p>{plugin.publisher} · v{plugin.version}</p>
          </div>
        </div>
        {plugin.featured && !plugin.installed && <span className={styles.featuredBadge}>{t("Featured")}</span>}
        {plugin.installed && <span className={styles.installedBadge}><AppIcon name="check" size={12} /> {t(plugin.system ? "System" : "Installed")}</span>}
      </div>

      <p className={styles.description}>{plugin.description || t("Extend CodeLocal with reusable tools and external data.")}</p>

      {categories.length > 0 && (
        <div className={styles.categories} aria-label={t("{name} categories", { name: plugin.name })}>
          {categories.map((category) => <span key={category}>{category}</span>)}
          {(plugin.executionTargets ?? []).map((target) => <span key={`runtime-${target}`}>{t(target === "local" ? "Local runtime" : "Cloud runtime")}</span>)}
        </div>
      )}

      <div className={styles.permissions}>
        <div className={styles.permissionTitle}><AppIcon name="shield" size={15} /><strong>{t("Permissions")}</strong></div>
        <div className={styles.permissionList}>
          {capabilities.length === 0
            ? <span>{t("No runtime permissions declared")}</span>
            : capabilities.map((capability) => <span key={capability}>{permissionLabels[capability] ? t(permissionLabels[capability]) : capability}</span>)}
        </div>
      </div>

      {plugin.installed && connections.length > 0 && (
        <div className={styles.connectionList}>
          {connections.map((connection) => (
            <div className={styles.connectionRow} key={`${connection.connection}-${connection.serverName}`}>
              <div className={styles.connectionCopy}>
                <strong>{connectionDeviceLabel(connection, workspaces)}</strong>
                <span>
                  {connection.state === "ready" ? t("{count} tools ready", { count: connection.toolCount }) : t(connection.state === "error" ? "Connection needs attention" : "Configured")}
                </span>
                {connection.lastError && <small title={connection.lastError}>{message(connection.lastError)}</small>}
              </div>
              <button className={styles.textButton} disabled={busy} onClick={() => void disconnect(plugin, connection)} type="button">{t("Disconnect")}</button>
            </div>
          ))}
        </div>
      )}

      {plugin.installed && showConfig && (
        <div className={styles.configPanel}>
          <div className={styles.configHeader}>
            <div>
              <strong>{t("Configure connection")}</strong>
              <span>{t(isPenpot ? "Connect this device to CodeLocal's hosted Penpot MCP." : "The MCP server is installed on your selected CodeLocal device.")}</span>
            </div>
            <button aria-label={t("Close Plugin configuration")} title={t("Close Plugin configuration")} disabled={busy} className={styles.iconButton} onClick={() => setShowConfig(false)} type="button"><AppIcon name="close" size={16} /></button>
          </div>
          <label className={styles.field}>
            <span>{t("Device / workspace")}</span>
            <select onChange={(event) => setSelectedWorkspace(event.target.value)} value={selectedWorkspace}>
              <option value="">{t("Select a running workspace")}</option>
              {onlineWorkspaces.map((workspace) => <option key={`${workspace.deviceId}-${workspace.workspaceId}`} value={workspaceValue(workspace)}>{workspaceLabel(workspace)}</option>)}
            </select>
          </label>
          <label className={styles.field}>
            <span>{t("MCP endpoint")}</span>
            <input autoComplete="off" inputMode="url" onChange={(event) => setEndpoint(event.target.value)} placeholder="https://example.com/mcp" readOnly={isPenpot} type="url" value={endpoint} />
          </label>
          <label className={styles.field}>
            <span>{isPenpot ? t("Penpot MCP key or copied server URL") : <>{t("Bearer token")} <em>{t("Optional")}</em></>}</span>
            <input autoCapitalize="none" autoComplete="new-password" disabled={Boolean(bearerEnv)} onChange={(event) => setBearerToken(event.target.value)} placeholder={t("Stored encrypted by CodeLocal")} spellCheck={false} type="password" value={bearerToken} />
          </label>
          {!isPenpot && <label className={styles.field}>
            <span>{t("Or local token environment variable")} <em>{t("Optional")}</em></span>
            <input autoCapitalize="none" autoComplete="off" disabled={Boolean(bearerToken)} onChange={(event) => setBearerEnv(event.target.value)} placeholder="GITHUB_TOKEN" spellCheck={false} value={bearerEnv} />
          </label>}
          <p className={styles.credentialNote}><AppIcon name="shield" size={13} />{t(isPenpot ? "Generate the key in Penpot under Account → Integrations → MCP Server. CodeLocal stores it encrypted and never writes it into your repository." : "Token values are encrypted server-side and materialized only for the selected runtime connection. Environment references remain local to your device.")}</p>
          {onlineWorkspaces.length === 0 && <p className={styles.configWarning}>{t("No running CodeLocal workspace found. Start {command} on a paired device first.", { command: "codelocal" })}</p>}
          <div className={styles.configActions}>
            <button className={styles.secondaryButton} disabled={busy} onClick={() => setShowConfig(false)} type="button">{t("Cancel")}</button>
            <button className={styles.primaryButton} disabled={busy || !selectedWorkspace || !endpoint.trim() || (isPenpot && connections.length === 0 && !bearerToken.trim())} onClick={() => void submitConnection()} type="button">{t(busy ? "Connecting…" : "Connect & test")}</button>
          </div>
        </div>
      )}

      <div className={styles.cardFooter}>
        <div className={styles.installState}>
          {plugin.installed && plugin.setupRequired && readyConnections > 0 && <span><span className={styles.statusDot} data-ready="true" />{t("{count} devices ready", { count: readyConnections })}</span>}
          {plugin.installed && plugin.setupRequired && readyConnections === 0 && hasConnectionError && <span><span className={styles.statusDot} data-error="true" />{t("Connection needs attention")}</span>}
          {plugin.installed && plugin.setupRequired && readyConnections === 0 && !hasConnectionError && <span><span className={styles.statusDot} />{t("Connection not configured")}</span>}
          {plugin.installed && !plugin.setupRequired && <span><span className={styles.statusDot} data-ready="true" />{t("Ready")}</span>}
          {!plugin.installed && <span>{t("Install to make this Plugin available to CodeLocal.")}</span>}
          {plugin.updateAvailable && <span className={styles.updateText}>{t("A newer manifest is available.")}</span>}
        </div>
        <div className={styles.actions}>
          {!plugin.installed && <button className={styles.primaryButton} disabled={busy} onClick={() => void install(plugin)} type="button">{t(busy ? "Installing…" : "Install")}</button>}
          {plugin.installed && plugin.updateAvailable && <button className={styles.primaryButton} disabled={busy} onClick={() => void install(plugin)} type="button">{t(busy ? "Updating…" : "Update")}</button>}
          {plugin.installed && plugin.setupRequired && <button className={styles.primaryButton} disabled={busy} onClick={openConfigure} type="button">{t(connections.length > 0 ? "Configure" : "Connect")}</button>}
          {plugin.installed && !plugin.system && <button className={styles.secondaryButton} disabled={busy || connections.length > 0} onClick={() => void uninstall(plugin)} title={connections.length > 0 ? t("Disconnect all devices before removing this Plugin") : undefined} type="button">{t("Remove")}</button>}
        </div>
      </div>
    </article>
  );
}

function EmptyState({ installed }: { installed: boolean }) {
  const { t } = useTranslations();
  return (
    <div className={styles.emptyState}>
      <AppIcon name="plugin" size={24} />
      <strong>{t(installed ? "No Plugins installed" : "No Plugins found")}</strong>
      <span>{t(installed ? "Install a Plugin from Explore and it will appear here." : "Try a different search term.")}</span>
    </div>
  );
}

export function PluginsHub() {
  const { locale, t, message } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  const [view, setView] = useState<PluginView>("explore");
  const [resource, setResource] = useState<PluginsResource>({ items: [], installedCount: 0 });
  const [workspaces, setWorkspaces] = useState<WorkspaceItem[]>([]);
  const [account, setAccount] = useState<AccountResource | null>(null);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [busyPlugin, setBusyPlugin] = useState("");
  const [notice, setNotice] = useState<Notice>(null);

  const load = useCallback(async () => {
    const [pluginsResponse, accountResponse, workspacesResponse] = await Promise.all([
      fetch("/api/v1/plugins", { credentials: "include", cache: "no-store" }),
      fetch("/api/v1/account", { credentials: "include", cache: "no-store" }),
      fetch("/api/v1/workspaces", { credentials: "include", cache: "no-store" }),
    ]);
    if (!pluginsResponse.ok) throw new Error(await responseError(pluginsResponse));
    if (!accountResponse.ok) throw new Error(await responseError(accountResponse));
    if (!workspacesResponse.ok) throw new Error(await responseError(workspacesResponse));
    const plugins = await pluginsResponse.json() as Partial<PluginsResource>;
    const nextAccount = await accountResponse.json() as AccountResource;
    const nextWorkspaces = await workspacesResponse.json() as unknown;
    if (!isWorkspacesResource(nextWorkspaces)) throw new Error("Workspace data is invalid.");
    setResource({
      items: Array.isArray(plugins.items) ? plugins.items : [],
      installedCount: typeof plugins.installedCount === "number" ? plugins.installedCount : 0,
    });
    setAccount(nextAccount);
    setWorkspaces(nextWorkspaces.items);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void load()
        .then(() => { if (!cancelled) setLoadFailed(false); })
        .catch(() => { if (!cancelled) setLoadFailed(true); })
        .finally(() => { if (!cancelled) setLoading(false); });
    }, 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [load]);

  const refresh = useCallback(async () => {
    await load();
    setLoadFailed(false);
  }, [load, setLoadFailed]);

  const mutationHeaders = useCallback((json = false) => {
    if (!account?.csrf) throw new Error("Security token unavailable. Reload the page and try again.");
    return {
      "X-CSRF-Token": account.csrf,
      ...(json ? { "Content-Type": "application/json" } : {}),
    };
  }, [account]);

  const mutateInstall = useCallback(async (plugin: PluginItem, method: "POST" | "DELETE") => {
    const response = await fetch(`/api/v1/plugins/${encodeURIComponent(plugin.id)}/install`, {
      method,
      credentials: "include",
      headers: mutationHeaders(),
    });
    if (!response.ok) throw new Error(await responseError(response));
  }, [mutationHeaders]);

  const install = useCallback(async (plugin: PluginItem) => {
    setBusyPlugin(plugin.id);
    setNotice(null);
    try {
      await mutateInstall(plugin, "POST");
      await refresh();
      setNotice({ kind: "success", text: "{name} installed.", values: { name: plugin.name } });
      setView("installed");
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin install failed." });
    } finally {
      setBusyPlugin("");
    }
  }, [mutateInstall, refresh]);

  const uninstall = useCallback(async (plugin: PluginItem) => {
    if (!window.confirm(t("Remove {name} from CodeLocal?", { name: plugin.name }))) return;
    setBusyPlugin(plugin.id);
    setNotice(null);
    try {
      await mutateInstall(plugin, "DELETE");
      await refresh();
      setNotice({ kind: "success", text: "{name} removed.", values: { name: plugin.name } });
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin removal failed." });
    } finally {
      setBusyPlugin("");
    }
  }, [mutateInstall, refresh, t]);

  const connect = useCallback(async (plugin: PluginItem, input: ConnectionInput) => {
    setBusyPlugin(plugin.id);
    setNotice(null);
    try {
      const response = await fetch(`/api/v1/plugins/${encodeURIComponent(plugin.id)}/connections`, {
        method: "POST",
        credentials: "include",
        headers: mutationHeaders(true),
        body: JSON.stringify(input),
      });
      if (!response.ok) throw new Error(await responseError(response));
      const payload = await response.json() as { connection?: PluginConnection };
      await refresh();
      if (payload.connection?.state === "ready") {
        setNotice({ kind: "success", text: "{name} connected. {count} tools discovered on the selected device.", values: { name: plugin.name, count: payload.connection.toolCount } });
        return true;
      } else {
        setNotice({ kind: "error", text: payload.connection?.lastError || "{name} was configured, but its MCP server is not ready yet.", values: { name: plugin.name } });
        return false;
      }
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin connection failed." });
      return false;
    } finally {
      setBusyPlugin("");
    }
  }, [mutationHeaders, refresh]);

  const disconnect = useCallback(async (plugin: PluginItem, connection: PluginConnection) => {
    if (!window.confirm(t("Disconnect {name} from this CodeLocal device?", { name: plugin.name }))) return;
    setBusyPlugin(plugin.id);
    setNotice(null);
    try {
      const response = await fetch(`/api/v1/plugins/${encodeURIComponent(plugin.id)}/connections/${encodeURIComponent(connection.deviceId)}`, {
        method: "DELETE",
        credentials: "include",
        headers: mutationHeaders(),
      });
      if (!response.ok) throw new Error(await responseError(response));
      await refresh();
      setNotice({ kind: "success", text: "{name} disconnected from the device.", values: { name: plugin.name } });
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin disconnect failed." });
    } finally {
      setBusyPlugin("");
    }
  }, [mutationHeaders, refresh, t]);

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return resource.items
      .filter((plugin) => view === "explore" || plugin.installed)
      .filter((plugin) => {
        if (!needle) return true;
        return [plugin.name, plugin.publisher, plugin.description, ...(plugin.categories ?? [])]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(needle));
      })
      .sort((left, right) => {
        if (left.installed !== right.installed) return left.installed ? -1 : 1;
        if (left.featured !== right.featured) return left.featured ? -1 : 1;
        return left.name.localeCompare(right.name, locale);
      });
  }, [resource.items, search, view, locale]);

  return (
    <section className={styles.page}>
      <h1 className="sr-only">{t("Plugins")}</h1>
      <div className={styles.toolbar}>
        <div className={styles.installCount}><AppIcon name="plugin" size={14} />{t("{count} installed", { count: resource.installedCount })}</div>
        <div className={styles.searchBox}>
          <AppIcon name="search" size={16} />
          <input aria-label={t("Search Plugins")} onChange={(event) => setSearch(event.target.value)} placeholder={t("Search Plugins")} type="search" value={search} />
        </div>
        <nav className={styles.tabs} aria-label={t("Plugin views")}>
          <button aria-pressed={view === "explore"} className={view === "explore" ? styles.activeTab : undefined} onClick={() => setView("explore")} type="button">{t("Explore")}</button>
          <button aria-pressed={view === "installed"} className={view === "installed" ? styles.activeTab : undefined} onClick={() => setView("installed")} type="button">{t("Installed")} <span>{number.format(resource.installedCount)}</span></button>
        </nav>
      </div>

      {account?.requiresReauthentication && <div className={styles.notice}>{t("Your session is old. Installing or connecting a new Plugin will ask you to sign in again.")}</div>}
      {notice && <div role="status" className={notice.kind === "error" ? styles.noticeError : styles.noticeSuccess}>{message(notice.text, notice.values)}</div>}

      <div className={styles.sectionIntro}>
        <div>
          <h2>{t(view === "explore" ? "Plugin Directory" : "Installed Plugins")}</h2>
          <p>{t(view === "explore" ? "Curated integrations with explicit permissions and immutable manifest versions." : "These Plugins are enabled for your CodeLocal account.")}</p>
        </div>
        <span>{t("{count} plugins", { count: visible.length })}</span>
      </div>

      <div className={styles.grid}>
        {loading && <div className={styles.emptyState}><AppIcon name="refresh" size={24} /><strong>{t("Loading Plugins")}</strong><span>{t("Reading your Plugin catalog…")}</span></div>}
        {!loading && loadFailed && <div className={styles.emptyState}><AppIcon name="plugin" size={24} /><strong>{t("Plugin catalog unavailable")}</strong><span>{t("The backend could not load your Plugin state.")}</span><button onClick={() => { setLoading(true); void refresh().catch(() => setLoadFailed(true)).finally(() => setLoading(false)); }} type="button">{t("Retry")}</button></div>}
        {!loading && !loadFailed && visible.map((plugin) => (
          <PluginCard busy={busyPlugin === plugin.id} connect={connect} disconnect={disconnect} install={install} key={plugin.id} plugin={plugin} uninstall={uninstall} workspaces={workspaces} />
        ))}
        {!loading && !loadFailed && visible.length === 0 && <EmptyState installed={view === "installed"} />}
      </div>
    </section>
  );
}
