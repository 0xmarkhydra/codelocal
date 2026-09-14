"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { AppIcon } from "../app-icon";
import styles from "./integrations.module.css";

type Device = {
  deviceId: string;
  deviceName: string;
  status: "online" | "offline" | "revoked";
};

type Server = {
  name: string;
  enabled: boolean;
  transport: "stdio" | "http";
  command?: string;
  args?: string[];
  url?: string;
};

type Connection = {
  target: "online" | "local";
  deviceId?: string;
  server: Server;
  state: "pending" | "configured" | "ready" | "error";
  toolCount: number;
  lastError?: string;
  connectedAt?: number;
  updatedAt: number;
};

type Account = { csrf: string };

type Notice = { kind: "success" | "error"; text: string } | null;

const ONLINE_SAMPLE = `{
  "mcpServers": {
    "github": {
      "url": "https://example.com/mcp"
    }
  }
}`;

const LOCAL_SAMPLE = `{
  "mcpServers": {
    "playwright": {
      "command": "npx",
      "args": ["-y", "@playwright/mcp@latest"]
    }
  }
}`;

function connectionStatus(item: Connection) {
  if (item.state === "ready") return { label: "Ready", className: styles.ready };
  if (item.state === "error") return { label: "Failed", className: styles.failed };
  return { label: "Pending", className: styles.pending };
}

function serverSummary(server: Server) {
  if (server.transport === "stdio") return [server.command, ...(server.args ?? [])].filter(Boolean).join(" ");
  return server.url || "Remote MCP";
}

function secretReferences(raw: string) {
  const found = new Set<string>();
  for (const match of raw.matchAll(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g)) found.add(match[1]);
  return [...found];
}

function deviceLabel(devices: Device[], id?: string) {
  return devices.find((device) => device.deviceId === id)?.deviceName || id || "Unknown device";
}

export function IntegrationsHub() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [account, setAccount] = useState<Account | null>(null);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [target, setTarget] = useState<"online" | "local">("online");
  const [deviceId, setDeviceId] = useState("");
  const [config, setConfig] = useState(ONLINE_SAMPLE);
  const [secrets, setSecrets] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);

  const refresh = useCallback(async () => {
    const [connectionsResponse, devicesResponse, accountResponse] = await Promise.all([
      fetch("/api/v1/mcp/connections", { credentials: "same-origin", cache: "no-store" }),
      fetch("/api/v1/devices", { credentials: "same-origin", cache: "no-store" }),
      fetch("/api/v1/account", { credentials: "same-origin", cache: "no-store" }),
    ]);
    if (!connectionsResponse.ok || !devicesResponse.ok || !accountResponse.ok) throw new Error("Integrations are temporarily unavailable.");
    const connectionPayload = await connectionsResponse.json() as { items?: Connection[] };
    const devicePayload = await devicesResponse.json() as { items?: Device[] };
    const accountPayload = await accountResponse.json() as Account;
    setConnections(Array.isArray(connectionPayload.items) ? connectionPayload.items : []);
    setDevices(Array.isArray(devicePayload.items) ? devicePayload.items.filter((item) => item.status !== "revoked") : []);
    setAccount(accountPayload);
    setDeviceId((current) => current || devicePayload.items?.find((item: Device) => item.status === "online")?.deviceId || devicePayload.items?.[0]?.deviceId || "");
  }, []);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        await refresh();
      } catch (error) {
        if (!cancelled) setNotice({ kind: "error", text: error instanceof Error ? error.message : "Integrations are temporarily unavailable." });
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [refresh]);

  const selectTarget = useCallback((next: "online" | "local") => {
    setTarget(next);
    setConfig(next === "online" ? ONLINE_SAMPLE : LOCAL_SAMPLE);
    setSecrets({});
  }, []);

  const refs = useMemo(() => secretReferences(config), [config]);
  const online = connections.filter((item) => item.target === "online");
  const local = connections.filter((item) => item.target === "local");
  const deviceGroups = devices.map((device) => ({ device, items: local.filter((item) => item.deviceId === device.deviceId) }));
  const orphanLocal = local.filter((item) => !devices.some((device) => device.deviceId === item.deviceId));

  async function addConnection() {
    if (!account?.csrf || !config.trim() || (target === "local" && !deviceId)) return;
    setSaving(true);
    setNotice(null);
    try {
      const response = await fetch("/api/v1/mcp/connections", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": account.csrf },
        body: JSON.stringify({ target, deviceId: target === "local" ? deviceId : undefined, config, secrets }),
      });
      const payload = await response.json().catch(() => ({})) as { detail?: string; error?: string };
      if (!response.ok) throw new Error(payload.detail || payload.error || "Could not add this MCP connection.");
      await refresh();
      setDialogOpen(false);
      setNotice({ kind: "success", text: target === "online" ? "Online MCP added and tested." : "Local MCP saved. Offline devices will apply it automatically when they reconnect." });
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Could not add this MCP connection." });
    } finally {
      setSaving(false);
    }
  }

  async function removeConnection(item: Connection) {
    if (!account?.csrf) return;
    const query = item.target === "local" && item.deviceId ? `?deviceId=${encodeURIComponent(item.deviceId)}` : "";
    const response = await fetch(`/api/v1/mcp/connections/${encodeURIComponent(item.target)}/${encodeURIComponent(item.server.name)}${query}`, {
      method: "DELETE",
      credentials: "same-origin",
      headers: { "X-CSRF-Token": account.csrf },
    });
    if (!response.ok) {
      setNotice({ kind: "error", text: "Could not remove this MCP connection." });
      return;
    }
    await refresh();
    setNotice({ kind: "success", text: "MCP connection removed." });
  }

  function ConnectionCard({ item }: { item: Connection }) {
    const status = connectionStatus(item);
    return (
      <article className={styles.connectionCard}>
        <div className={styles.connectionTop}>
          <div className={styles.logoBox}><AppIcon name={item.target === "online" ? "connection" : "terminal"} size={19} /></div>
          <div className={styles.connectionTitle}><strong>{item.server.name}</strong><span>{serverSummary(item.server)}</span></div>
          <span className={`${styles.status} ${status.className}`}><i />{status.label}</span>
        </div>
        <div className={styles.cardMeta}>
          <span>{item.toolCount} tools</span>
          {item.target === "local" && <span>{deviceLabel(devices, item.deviceId)}</span>}
        </div>
        {item.lastError && <p className={styles.errorText}>{item.lastError}</p>}
        <div className={styles.cardActions}>
          <button type="button" className={styles.ghostButton} onClick={() => void removeConnection(item)}>Remove</button>
        </div>
      </article>
    );
  }

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <div><span className={styles.eyebrow}>MCP</span><h1>Integrations</h1><p>Connect tools and services once. CodeLocal handles where they run.</p></div>
        <div className={styles.headerActions}><Link href="/dashboard/plugins" className={styles.secondaryButton}>Explore plugins</Link><button className={styles.primaryButton} type="button" onClick={() => setDialogOpen(true)}><AppIcon name="plus" size={16} /> Add MCP</button></div>
      </header>

      {notice && <div className={`${styles.notice} ${notice.kind === "error" ? styles.noticeError : styles.noticeSuccess}`}>{notice.text}</div>}

      <section className={styles.section}>
        <div className={styles.sectionHeader}><div className={styles.sectionIcon}><AppIcon name="connection" size={19} /></div><div><h2>Online</h2><p>Works even when your computer is off.</p></div></div>
        <div className={styles.grid}>{online.map((item) => <ConnectionCard item={item} key={`online-${item.server.name}`} />)}{!loading && online.length === 0 && <div className={styles.empty}>No online MCPs yet. Add a remote MCP URL or paste its JSON config.</div>}</div>
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHeader}><div className={styles.sectionIcon}><AppIcon name="device" size={19} /></div><div><h2>This device</h2><p>Local MCPs live on the selected computer and sync automatically from the web.</p></div></div>
        <div className={styles.deviceStack}>
          {deviceGroups.map(({ device, items }) => <section className={styles.deviceGroup} key={device.deviceId}>
            <div className={styles.deviceHeader}><div><AppIcon name="device" size={17} /><strong>{device.deviceName}</strong></div><span className={device.status === "online" ? styles.deviceOnline : styles.deviceOffline}><i />{device.status === "online" ? "Online" : "Offline"}</span></div>
            <div className={styles.grid}>{items.map((item) => <ConnectionCard item={item} key={`${device.deviceId}-${item.server.name}`} />)}{items.length === 0 && <div className={styles.emptySmall}>No local MCPs on this device.</div>}</div>
          </section>)}
          {orphanLocal.length > 0 && <section className={styles.deviceGroup}><div className={styles.deviceHeader}><strong>Unavailable device</strong></div><div className={styles.grid}>{orphanLocal.map((item) => <ConnectionCard item={item} key={`${item.deviceId}-${item.server.name}`} />)}</div></section>}
          {!loading && devices.length === 0 && <div className={styles.empty}>Pair a CodeLocal device before adding a local MCP.</div>}
        </div>
      </section>

      {dialogOpen && <div className={styles.backdrop} role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target && !saving) setDialogOpen(false); }}>
        <section className={styles.modal} role="dialog" aria-modal="true" aria-labelledby="add-mcp-title">
          <div className={styles.modalHeader}><div><h2 id="add-mcp-title">Add MCP</h2><p>Paste the same JSON you would use in Cursor or Claude.</p></div><button className={styles.iconButton} type="button" disabled={saving} onClick={() => setDialogOpen(false)} aria-label="Close"><AppIcon name="close" size={18} /></button></div>
          <div className={styles.targetGrid}>
            <button type="button" className={`${styles.targetCard} ${target === "online" ? styles.targetActive : ""}`} onClick={() => selectTarget("online")}><AppIcon name="connection" size={21} /><strong>Run Online</strong><span>Computer can be off</span></button>
            <button type="button" className={`${styles.targetCard} ${target === "local" ? styles.targetActive : ""}`} onClick={() => selectTarget("local")}><AppIcon name="device" size={21} /><strong>Run on my device</strong><span>Uses CodeLocal client</span></button>
          </div>
          {target === "local" && <label className={styles.field}><span>Device</span><select value={deviceId} onChange={(event) => setDeviceId(event.target.value)}>{devices.map((device) => <option key={device.deviceId} value={device.deviceId}>{device.deviceName} · {device.status}</option>)}</select></label>}
          <label className={styles.field}><span>MCP JSON</span><textarea className={styles.codeInput} value={config} onChange={(event) => setConfig(event.target.value)} spellCheck={false} /></label>
          {refs.length > 0 && <div className={styles.secretPanel}><div><strong>Secrets referenced by this config</strong><p>{target === "local" ? "Optional: leave blank to use the environment variable already configured on this device." : "Values are encrypted. They are never written into the MCP JSON."}</p></div>{refs.map((name) => <label className={styles.field} key={name}><span>{name}</span><input type="password" autoComplete="off" value={secrets[name] ?? ""} placeholder={target === "local" ? `Optional value for ${name}` : `Value for ${name}`} onChange={(event) => setSecrets((current) => ({ ...current, [name]: event.target.value }))} /></label>)}</div>}
          <div className={styles.review}><div><AppIcon name={target === "online" ? "connection" : "device"} size={17} /><span><strong>Runs on</strong>{target === "online" ? "CodeLocal Cloud" : deviceLabel(devices, deviceId)}</span></div><div><AppIcon name="shield" size={17} /><span><strong>Verification</strong>{target === "online" ? "Handshake + tools/list before Ready" : "Persist locally + probe before Ready"}</span></div></div>
          <div className={styles.modalFooter}><button className={styles.secondaryButton} type="button" disabled={saving} onClick={() => setDialogOpen(false)}>Cancel</button><button className={styles.primaryButton} type="button" disabled={saving || !config.trim() || (target === "local" && !deviceId) || target === "online" && refs.some((ref) => !secrets[ref])} onClick={() => void addConnection()}>{saving ? "Testing…" : "Test & add MCP"}</button></div>
        </section>
      </div>}
    </section>
  );
}
