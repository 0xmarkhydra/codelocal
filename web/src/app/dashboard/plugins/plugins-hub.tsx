"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AppIcon } from "../app-icon";
import styles from "./plugins.module.css";

type PluginView = "explore" | "installed";

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
};

type PluginsResource = {
  items: PluginItem[];
  installedCount: number;
};

type AccountResource = {
  csrf: string;
  requiresReauthentication: boolean;
};

type Notice = { kind: "success" | "error"; text: string } | null;

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

function responseError(response: Response) {
  return response.json()
    .then((payload: { detail?: string; error?: string }) => {
      if (payload.detail) return payload.detail;
      switch (payload.error) {
        case "invalid_csrf": return "Security token expired. Reload the page and try again.";
        case "reauthentication_required": return "Sign in again before installing a new Plugin.";
        case "plugin_not_found": return "This Plugin is no longer available.";
        case "plugin_install_failed": return "CodeLocal could not save the Plugin installation.";
        case "plugin_uninstall_failed": return "CodeLocal could not remove the Plugin installation.";
        default: return `Request failed (${response.status}).`;
      }
    })
    .catch(() => `Request failed (${response.status}).`);
}

function capabilityLabel(value: string) {
  return permissionLabels[value] ?? value.replaceAll("_", " ");
}

function PluginCard({
  plugin,
  busy,
  install,
  uninstall,
}: {
  plugin: PluginItem;
  busy: boolean;
  install: (plugin: PluginItem) => Promise<void>;
  uninstall: (plugin: PluginItem) => Promise<void>;
}) {
  const capabilities = plugin.capabilities ?? [];
  const categories = (plugin.categories ?? []).slice(0, 3);
  const monogram = plugin.name.slice(0, 2).toUpperCase();

  return (
    <article className={styles.pluginCard} data-installed={plugin.installed || undefined}>
      <div className={styles.cardTop}>
        <div className={styles.pluginIdentity}>
          <div className={styles.pluginIcon} aria-hidden="true">{monogram}</div>
          <div>
            <div className={styles.nameRow}>
              <h2>{plugin.name}</h2>
              {plugin.publisherVerified && <span className={styles.verified} title="Verified publisher"><AppIcon name="check" size={11} /> Verified</span>}
            </div>
            <p>{plugin.publisher} · v{plugin.version}</p>
          </div>
        </div>
        {plugin.featured && !plugin.installed && <span className={styles.featuredBadge}>Featured</span>}
        {plugin.installed && <span className={styles.installedBadge}><AppIcon name="check" size={12} /> Installed</span>}
      </div>

      <p className={styles.description}>{plugin.description || "Extend CodeLocal with reusable tools and external data."}</p>

      {categories.length > 0 && (
        <div className={styles.categories} aria-label={`${plugin.name} categories`}>
          {categories.map((category) => <span key={category}>{category}</span>)}
        </div>
      )}

      <div className={styles.permissions}>
        <div className={styles.permissionTitle}><AppIcon name="shield" size={15} /><strong>Permissions</strong></div>
        <div className={styles.permissionList}>
          {capabilities.length === 0
            ? <span>No runtime permissions declared</span>
            : capabilities.slice(0, 4).map((capability) => <span key={capability}>{capabilityLabel(capability)}</span>)}
          {capabilities.length > 4 && <span>+{capabilities.length - 4} more</span>}
        </div>
      </div>

      <div className={styles.cardFooter}>
        <div className={styles.installState}>
          {plugin.installed && plugin.setupRequired && <span><span className={styles.statusDot} />Connection not configured</span>}
          {plugin.installed && !plugin.setupRequired && <span><span className={styles.statusDot} data-ready="true" />Ready</span>}
          {!plugin.installed && <span>Install to make this Plugin available to CodeLocal.</span>}
          {plugin.updateAvailable && <span className={styles.updateText}>A newer manifest is available.</span>}
        </div>
        <div className={styles.actions}>
          {!plugin.installed && <button className={styles.primaryButton} disabled={busy} onClick={() => void install(plugin)} type="button">{busy ? "Installing…" : "Install"}</button>}
          {plugin.installed && plugin.updateAvailable && <button className={styles.primaryButton} disabled={busy} onClick={() => void install(plugin)} type="button">{busy ? "Updating…" : "Update"}</button>}
          {plugin.installed && <button className={styles.secondaryButton} disabled={busy} onClick={() => void uninstall(plugin)} type="button">Remove</button>}
        </div>
      </div>
    </article>
  );
}

function EmptyState({ installed }: { installed: boolean }) {
  return (
    <div className={styles.emptyState}>
      <AppIcon name="plugin" size={24} />
      <strong>{installed ? "No Plugins installed" : "No Plugins found"}</strong>
      <span>{installed ? "Install a Plugin from Explore and it will appear here." : "Try a different search term."}</span>
    </div>
  );
}

export function PluginsHub() {
  const [view, setView] = useState<PluginView>("explore");
  const [resource, setResource] = useState<PluginsResource>({ items: [], installedCount: 0 });
  const [account, setAccount] = useState<AccountResource | null>(null);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [busyPlugin, setBusyPlugin] = useState("");
  const [notice, setNotice] = useState<Notice>(null);

  const load = useCallback(async () => {
    const [pluginsResponse, accountResponse] = await Promise.all([
      fetch("/api/v1/plugins", { credentials: "include", cache: "no-store" }),
      fetch("/api/v1/account", { credentials: "include", cache: "no-store" }),
    ]);
    if (!pluginsResponse.ok) throw new Error(await responseError(pluginsResponse));
    if (!accountResponse.ok) throw new Error(await responseError(accountResponse));
    const plugins = await pluginsResponse.json() as Partial<PluginsResource>;
    const nextAccount = await accountResponse.json() as AccountResource;
    setResource({
      items: Array.isArray(plugins.items) ? plugins.items : [],
      installedCount: typeof plugins.installedCount === "number" ? plugins.installedCount : 0,
    });
    setAccount(nextAccount);
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
  }, [load]);

  const mutate = useCallback(async (plugin: PluginItem, method: "POST" | "DELETE") => {
    if (!account?.csrf) throw new Error("Security token unavailable. Reload the page and try again.");
    const response = await fetch(`/api/v1/plugins/${encodeURIComponent(plugin.id)}/install`, {
      method,
      credentials: "include",
      headers: { "X-CSRF-Token": account.csrf },
    });
    if (!response.ok) throw new Error(await responseError(response));
  }, [account]);

  const install = useCallback(async (plugin: PluginItem) => {
    setBusyPlugin(plugin.id);
    setNotice(null);
    try {
      await mutate(plugin, "POST");
      await refresh();
      setNotice({ kind: "success", text: `${plugin.name} installed. Connection setup is kept separate from installation permissions.` });
      setView("installed");
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin install failed." });
    } finally {
      setBusyPlugin("");
    }
  }, [mutate, refresh]);

  const uninstall = useCallback(async (plugin: PluginItem) => {
    if (!window.confirm(`Remove ${plugin.name} from CodeLocal?`)) return;
    setBusyPlugin(plugin.id);
    setNotice(null);
    try {
      await mutate(plugin, "DELETE");
      await refresh();
      setNotice({ kind: "success", text: `${plugin.name} removed.` });
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Plugin removal failed." });
    } finally {
      setBusyPlugin("");
    }
  }, [mutate, refresh]);

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
        return left.name.localeCompare(right.name);
      });
  }, [resource.items, search, view]);

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>Extend CodeLocal</p>
          <h1>Plugins</h1>
          <p className={styles.lede}>Install capabilities once, then use them naturally from chat.</p>
        </div>
        <div className={styles.installCount}><AppIcon name="plugin" size={14} />{resource.installedCount} installed</div>
      </header>

      <div className={styles.toolbar}>
        <div className={styles.searchBox}>
          <AppIcon name="search" size={16} />
          <input aria-label="Search Plugins" onChange={(event) => setSearch(event.target.value)} placeholder="Search plugins" type="search" value={search} />
        </div>
        <nav className={styles.tabs} aria-label="Plugin views">
          <button aria-current={view === "explore" ? "page" : undefined} className={view === "explore" ? styles.activeTab : undefined} onClick={() => setView("explore")} type="button">Explore</button>
          <button aria-current={view === "installed" ? "page" : undefined} className={view === "installed" ? styles.activeTab : undefined} onClick={() => setView("installed")} type="button">Installed <span>{resource.installedCount}</span></button>
        </nav>
      </div>

      {account?.requiresReauthentication && <div className={styles.notice}>Your session is old. Installing a new Plugin will ask you to sign in again.</div>}
      {notice && <div className={notice.kind === "error" ? styles.noticeError : styles.noticeSuccess}>{notice.text}</div>}

      <div className={styles.sectionIntro}>
        <div>
          <h2>{view === "explore" ? "Plugin Directory" : "Installed Plugins"}</h2>
          <p>{view === "explore" ? "Curated integrations with explicit permissions and immutable manifest versions." : "These Plugins are enabled for your CodeLocal account."}</p>
        </div>
        <span>{visible.length} {visible.length === 1 ? "plugin" : "plugins"}</span>
      </div>

      <div className={styles.grid}>
        {loading && <div className={styles.emptyState}><AppIcon name="refresh" size={24} /><strong>Loading Plugins</strong><span>Reading your Plugin catalog…</span></div>}
        {!loading && loadFailed && <div className={styles.emptyState}><AppIcon name="plugin" size={24} /><strong>Plugin catalog unavailable</strong><span>The backend could not load your Plugin state.</span><button onClick={() => { setLoading(true); void refresh().finally(() => setLoading(false)); }} type="button">Retry</button></div>}
        {!loading && !loadFailed && visible.map((plugin) => (
          <PluginCard busy={busyPlugin === plugin.id} install={install} key={plugin.id} plugin={plugin} uninstall={uninstall} />
        ))}
        {!loading && !loadFailed && visible.length === 0 && <EmptyState installed={view === "installed"} />}
      </div>
    </section>
  );
}
