"use client";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageValues } from "@/lib/i18n/messages";

import { useCallback, useEffect, useMemo, useState } from "react";
import { isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { AppIcon } from "../app-icon";
import { PluginConnectDialog } from "./plugin-connect-dialog";
import { CustomMCPPanel } from "./custom-mcp-panel";
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
  executionTarget: "local" | "cloud";
  authKind?: "none" | "bearer" | "oauth";
};

type Notice = { kind: "success" | "error"; text: string; values?: MessageValues } | null;

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

function connectionDeviceLabel(connection: PluginConnection, workspaces: WorkspaceItem[], cloudLabel: string) {
  if (connection.executionTarget === "cloud") return cloudLabel;
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
  install: (plugin: PluginItem) => Promise<boolean>;
  uninstall: (plugin: PluginItem) => Promise<void>;
  connect: (plugin: PluginItem, input: ConnectionInput) => Promise<boolean>;
  disconnect: (plugin: PluginItem, connection: PluginConnection) => Promise<void>;
}) {
  const { t } = useTranslations();
  const capabilities = plugin.capabilities ?? [];
  const connections = plugin.connections ?? [];
  const monogram = plugin.name.slice(0, 2).toUpperCase();
  const [dialogOpen, setDialogOpen] = useState(false);
  const cloudReady = connections.find((connection) => connection.executionTarget === "cloud" && connection.state === "ready");
  const deviceReady = connections.find((connection) => connection.executionTarget !== "cloud" && connection.state === "ready");
  const configuredConnection = connections.find((connection) => connection.state === "configured");
  const hasConnectionError = connections.some((connection) => connection.state === "error");
  const connected = Boolean(cloudReady || deviceReady);
  const closeDialog = useCallback(() => setDialogOpen(false), []);

  const readyConnection = cloudReady ?? deviceReady;
  const status = cloudReady
    ? t("CodeLocal Cloud")
    : deviceReady
      ? connectionDeviceLabel(deviceReady, workspaces, t("CodeLocal Cloud"))
      : hasConnectionError
        ? t("Connection needs attention")
        : configuredConnection
          ? t("Configured")
          : t("Not connected");

  function openDialog() {
    setDialogOpen(true);
  }

  return (
    <>
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
        </div>

        <p className={styles.description}>{plugin.description || t("Extend CodeLocal with reusable tools and external data.")}</p>

        {!plugin.installed && (
          <div className={styles.permissionsSummary}>
            <AppIcon name="shield" size={13} />
            <span>{capabilities.length === 0
              ? t("No runtime permissions declared")
              : t("Requires {count} permissions", { count: capabilities.length })}</span>
          </div>
        )}

        <div className={styles.statusRow}>
          {plugin.installed ? (
            <div className={styles.statusCopy}>
              <span className={styles.statusMain}>
                <span
                  aria-hidden="true"
                  className={styles.statusDot}
                  data-error={hasConnectionError && !connected ? "true" : undefined}
                  data-ready={connected ? "true" : undefined}
                />
                {status}
              </span>
              {readyConnection && <span className={styles.statusHint}>{t("{count} tools last discovered", { count: readyConnection.toolCount })}</span>}
              {plugin.updateAvailable && <span className={styles.statusHint}>{t("New version available")}</span>}
            </div>
          ) : (
            <span className={styles.statusHint}>{t("Install to make this Plugin available to CodeLocal.")}</span>
          )}
          <button className={styles.primaryButton} disabled={busy} onClick={openDialog} type="button">
            {t(!plugin.installed ? "Install" : connections.length > 0 ? "Manage" : "Configure")}
          </button>
        </div>
      </article>
      {dialogOpen && (
        <PluginConnectDialog
          busy={busy}
          key={plugin.id}
          connect={connect}
          disconnect={disconnect}
          install={install}
          onClose={closeDialog}
          plugin={plugin}
          uninstall={uninstall}
          workspaces={workspaces}
        />
      )}
    </>
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
  const [view, setView] = useState<PluginView>("installed");
  const [resource, setResource] = useState<PluginsResource>({ items: [], installedCount: 0 });
  const [customMCPCount, setCustomMCPCount] = useState(0);
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
      return true;
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin install failed." });
      return false;
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
      if (input.authKind === "oauth" && input.executionTarget === "cloud") {
        const response = await fetch(`/api/v1/plugins/${encodeURIComponent(plugin.id)}/oauth`, { method: "POST", credentials: "same-origin", headers: mutationHeaders(true), body: JSON.stringify({ endpoint: input.endpoint }) });
        if (!response.ok) throw new Error(await responseError(response));
        const payload = await response.json() as { authorizationUrl?: unknown };
        if (typeof payload.authorizationUrl !== "string" || new URL(payload.authorizationUrl).protocol !== "https:") throw new Error("Invalid OAuth authorization URL");
        window.location.assign(payload.authorizationUrl);
        return true;
      }
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
        setNotice({ kind: "success", text: "{name} connected. {count} tools discovered.", values: { name: plugin.name, count: payload.connection.toolCount } });
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
    if (!window.confirm(t("Disconnect {name} from this connection?", { name: plugin.name }))) return;
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
      setNotice({ kind: "success", text: "{name} disconnected.", values: { name: plugin.name } });
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
      <header className={styles.header}>
        <h1>{t("Plugins")}</h1>
      </header>

      <div className={styles.toolbar}>
        <nav className={styles.tabs} aria-label={t("Plugin views")}>
          <button aria-pressed={view === "installed"} className={view === "installed" ? styles.activeTab : undefined} onClick={() => setView("installed")} type="button">
            {t("Installed")} <span>{number.format(resource.installedCount + customMCPCount)}</span>
          </button>
          <button aria-pressed={view === "explore"} className={view === "explore" ? styles.activeTab : undefined} onClick={() => setView("explore")} type="button">
            {t("Explore")}
          </button>
        </nav>
        <div className={styles.searchBox}>
          <AppIcon name="search" size={16} />
          <input aria-label={t("Search Plugins")} onChange={(event) => setSearch(event.target.value)} placeholder={t("Search Plugins")} type="search" value={search} />
        </div>
      </div>

      {account?.requiresReauthentication && <div className={styles.notice}>{t("Your session is old. Installing or connecting a new Plugin will ask you to sign in again.")}</div>}
      {notice && <div role="status" className={notice.kind === "error" ? styles.noticeError : styles.noticeSuccess}>{message(notice.text, notice.values)}</div>}

      {view === "installed" && <CustomMCPPanel onCountChange={setCustomMCPCount} />}

      <div className={styles.sectionIntro}>
        <h2>{t(view === "explore" ? "Plugin Directory" : "From Explore")}</h2>
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
