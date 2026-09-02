"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import styles from "./pool.module.css";

type ModelSource = { sourceId: string; source: string; kind: string; state: string; retryAt?: string; lastError?: string };
type Model = { id: string; active: boolean; state: string; availableSources: number; totalSources: number; sources?: ModelSource[] };
type Source = { id: string; name: string; kind: string; baseUrl: string; priority: number; enabled: boolean; managedBy: string };
type Snapshot = { models: Model[]; sources: Source[]; storageConfigured: boolean };

async function readJSON(response: Response) {
  const data = await response.json().catch(() => ({})) as Record<string, unknown>;
  if (!response.ok) throw new Error(typeof data.detail === "string" ? data.detail : typeof data.error === "string" ? data.error : `Request failed (${response.status})`);
  return data;
}

function sourceLabel(state: string) {
  switch (state) {
    case "healthy": return "Healthy";
    case "degraded": return "Degraded";
    case "exhausted": return "Limit reached";
    case "unauthorized": return "Credential invalid";
    case "cooldown": return "Cooling down";
    case "disabled": return "Disabled";
    default: return state;
  }
}

export function PoolDashboard() {
  const router = useRouter();
  const [snapshot, setSnapshot] = useState<Snapshot>({ models: [], sources: [], storageConfigured: false });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [name, setName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [priority, setPriority] = useState("50");

  const refresh = useCallback(async () => {
    setError("");
    try {
      const [modelsResponse, sourcesResponse] = await Promise.all([
        fetch("/api/pool/models", { cache: "no-store" }),
        fetch("/api/pool/sources", { cache: "no-store" }),
      ]);
      if (modelsResponse.status === 401 || sourcesResponse.status === 401) {
        router.replace("/login");
        return;
      }
      const modelsData = await readJSON(modelsResponse) as { models?: Model[] };
      const sourcesData = await readJSON(sourcesResponse) as { sources?: Source[]; storageConfigured?: boolean };
      setSnapshot({
        models: Array.isArray(modelsData.models) ? modelsData.models : [],
        sources: Array.isArray(sourcesData.sources) ? sourcesData.sources : [],
        storageConfigured: sourcesData.storageConfigured === true,
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Pool unavailable");
    } finally {
      setLoading(false);
    }
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => { void refresh(); }, 0);
    return () => window.clearTimeout(timer);
  }, [refresh]);

  const active = useMemo(() => snapshot.models.filter((model) => model.active), [snapshot.models]);
  const exhausted = useMemo(() => snapshot.models.filter((model) => !model.active), [snapshot.models]);

  async function addSource(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy("create");
    setError("");
    try {
      const response = await fetch("/api/pool/sources", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name, kind: "openai-compatible", baseUrl, apiKey, priority: Number(priority) || 50 }),
      });
      await readJSON(response);
      setName(""); setBaseUrl(""); setApiKey(""); setPriority("50");
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not add source");
    } finally { setBusy(""); }
  }

  async function setSourceEnabled(source: Source, enabled: boolean) {
    setBusy(`toggle:${source.id}`); setError("");
    try {
      await readJSON(await fetch(`/api/pool/sources/${encodeURIComponent(source.id)}`, {
        method: "PATCH", headers: { "content-type": "application/json" }, body: JSON.stringify({ enabled }),
      }));
      await refresh();
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not update source"); }
    finally { setBusy(""); }
  }

  async function removeSource(source: Source) {
    if (!window.confirm(`Xoá source “${source.name}”?`)) return;
    setBusy(`delete:${source.id}`); setError("");
    try {
      await readJSON(await fetch(`/api/pool/sources/${encodeURIComponent(source.id)}`, { method: "DELETE" }));
      await refresh();
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Could not delete source"); }
    finally { setBusy(""); }
  }

  async function logout() {
    await fetch("/api/session/logout", { method: "POST" });
    router.replace("/login");
    router.refresh();
  }

  return (
    <main className={styles.shell}>
      <header className={styles.topbar}>
        <div><span className="eyebrow">CodeLocal infrastructure</span><h1>Pool</h1><p>Canonical model gateway · 9Router engine · BYOK failover</p></div>
        <div className={styles.actions}><button onClick={() => void refresh()} type="button">Refresh</button><button onClick={() => void logout()} type="button">Logout</button></div>
      </header>

      <section className={styles.metrics}>
        <article><span>Active models</span><strong>{active.length}</strong></article>
        <article><span>Exhausted</span><strong>{exhausted.length}</strong></article>
        <article><span>Sources</span><strong>{snapshot.sources.length}</strong></article>
        <article><span>BYOK storage</span><strong>{snapshot.storageConfigured ? "Ready" : "Off"}</strong></article>
      </section>

      {error ? <div className={styles.alert}>{error}</div> : null}
      {loading ? <div className={styles.loading}>Đang tải trạng thái Pool…</div> : null}

      <section className={styles.grid}>
        <article className={styles.panel}>
          <header><div><span className="eyebrow">Client contract</span><h2>Canonical models</h2></div><small><code>GET /v1/models</code> chỉ trả model Active</small></header>
          <div className={styles.list}>
            {snapshot.models.map((model) => (
              <div className={styles.modelRow} data-active={model.active} key={model.id}>
                <span className={styles.dot} /><div className={styles.identity}><code>{model.id}</code><small>{model.availableSources}/{model.totalSources} source available</small></div>
                <div className={styles.chips}>{model.sources?.slice(0, 4).map((source) => <span data-state={source.state} key={`${model.id}:${source.sourceId}`}>{source.source} · {sourceLabel(source.state)}</span>)}</div>
                <strong>{model.active ? model.state === "degraded" ? "Degraded" : "Active" : "Exhausted"}</strong>
              </div>
            ))}
            {!snapshot.models.length && !loading ? <p className={styles.empty}>Chưa có canonical model nào.</p> : null}
          </div>
        </article>

        <article className={styles.panel}>
          <header><div><span className="eyebrow">Routing layer</span><h2>Sources</h2></div><small>Priority thấp hơn được thử trước</small></header>
          <div className={styles.list}>
            {snapshot.sources.map((source) => (
              <div className={styles.sourceRow} key={source.id}>
                <div className={styles.identity}><strong>{source.name}</strong><small>{source.kind} · priority {source.priority}</small><code>{source.baseUrl}</code></div>
                <span className={styles.badge} data-enabled={source.enabled}>{source.enabled ? "Enabled" : "Disabled"}</span>
                {source.managedBy === "pool" ? <div className={styles.rowActions}><button disabled={busy !== ""} onClick={() => void setSourceEnabled(source, !source.enabled)}>{source.enabled ? "Disable" : "Enable"}</button><button disabled={busy !== ""} onClick={() => void removeSource(source)}>Delete</button></div> : <small className={styles.managed}>Environment managed</small>}
              </div>
            ))}
          </div>
        </article>
      </section>

      {snapshot.storageConfigured ? <section className={styles.panel}>
        <header><div><span className="eyebrow">Bring your own key</span><h2>Add OpenAI-compatible source</h2></div><small>Credential được mã hoá AES-GCM trong Pool API</small></header>
        <form className={styles.form} onSubmit={addSource}>
          <label><span>Name</span><input maxLength={120} onChange={(event) => setName(event.target.value)} required value={name} /></label>
          <label><span>Base URL</span><input onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://api.example.com/v1" required type="url" value={baseUrl} /></label>
          <label><span>Credential</span><input autoComplete="off" onChange={(event) => setApiKey(event.target.value)} required type="password" value={apiKey} /></label>
          <label><span>Priority</span><input min="0" max="10000" onChange={(event) => setPriority(event.target.value)} required type="number" value={priority} /></label>
          <button disabled={busy !== ""} type="submit">{busy === "create" ? "Validating…" : "Validate & add"}</button>
        </form>
      </section> : null}

      <footer className={styles.footer}><span>Client → <code>pool.codelocal.cloud/v1</code></span><span>Pool API → canonical routing</span><span>9Router/BYOK → provider execution</span></footer>
    </main>
  );
}
