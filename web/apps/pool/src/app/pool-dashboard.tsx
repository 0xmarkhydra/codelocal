"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import styles from "./pool.module.css";

type ModelSource = {
  sourceId: string;
  source: string;
  kind: string;
  provider?: string;
  upstream?: string;
  state: string;
  availableRoutes?: number;
  totalRoutes?: number;
  quotaRemainingPercent?: number;
  quotaResetAt?: string;
  retryAt?: string;
  lastCheckedAt?: string;
  lastError?: string;
};

type Model = {
  id: string;
  active: boolean;
  state: string;
  availableSources: number;
  totalSources: number;
  availableRoutes?: number;
  totalRoutes?: number;
  sources?: ModelSource[];
};

type Source = { id: string; name: string; kind: string; baseUrl: string; priority: number; enabled: boolean; managedBy: string };
type Snapshot = { models: Model[]; sources: Source[]; storageConfigured: boolean };
type TestResult = { model: string; ok: boolean; httpStatus?: number; source?: string; provider?: string; upstream?: string; latencyMs?: number; attempts: number; error?: string };

async function readJSON(response: Response) {
  const data = await response.json().catch(() => ({})) as Record<string, unknown>;
  if (!response.ok) throw new Error(typeof data.detail === "string" ? data.detail : typeof data.error === "string" ? data.error : `Request failed (${response.status})`);
  return data;
}

function sourceLabel(state: string) {
  switch (state) {
    case "healthy": return "Healthy";
    case "degraded": return "Degraded";
    case "exhausted": return "Exhausted";
    case "unauthorized": return "Unauthorized";
    case "cooldown": return "Cooldown";
    case "unavailable": return "Unavailable";
    case "disabled": return "Disabled";
    default: return state || "Unknown";
  }
}

function modelLabel(model: Model) {
  if (model.active && model.state === "degraded") return "Degraded";
  if (model.active) return "Active";
  return sourceLabel(model.state);
}

function quotaLabel(source?: ModelSource) {
  if (typeof source?.quotaRemainingPercent !== "number") return "—";
  return `${Math.max(0, Math.min(100, Math.round(source.quotaRemainingPercent)))}%`;
}

function resetLabel(source?: ModelSource) {
  const raw = source?.quotaResetAt || source?.retryAt;
  if (!raw) return "—";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("vi-VN", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function primaryRoute(model: Model) {
  return model.sources?.find((source) => source.state === "healthy") || model.sources?.find((source) => source.state === "degraded") || model.sources?.[0];
}

export function PoolDashboard() {
  const router = useRouter();
  const [snapshot, setSnapshot] = useState<Snapshot>({ models: [], sources: [], storageConfigured: false });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [tests, setTests] = useState<Record<string, TestResult>>({});
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
  const degraded = useMemo(() => snapshot.models.filter((model) => model.active && model.state === "degraded"), [snapshot.models]);
  const unavailable = useMemo(() => snapshot.models.filter((model) => !model.active), [snapshot.models]);
  const totalRoutes = useMemo(() => snapshot.models.reduce((sum, model) => sum + (model.totalRoutes || model.totalSources || 0), 0), [snapshot.models]);
  const availableRoutes = useMemo(() => snapshot.models.reduce((sum, model) => sum + (model.availableRoutes || model.availableSources || 0), 0), [snapshot.models]);
  const visibleModels = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return snapshot.models;
    return snapshot.models.filter((model) => model.id.toLowerCase().includes(needle) || model.sources?.some((source) => `${source.provider || ""} ${source.upstream || ""}`.toLowerCase().includes(needle)));
  }, [query, snapshot.models]);

  async function testModel(model: Model) {
    setBusy(`test:${model.id}`);
    setError("");
    try {
      const response = await fetch(`/api/pool/models/${model.id}/test`, { method: "POST" });
      const result = await readJSON(response) as unknown as TestResult;
      setTests((current) => ({ ...current, [model.id]: result }));
      await refresh();
    } catch (cause) {
      setTests((current) => ({ ...current, [model.id]: { model: model.id, ok: false, attempts: 0, error: cause instanceof Error ? cause.message : "Test failed" } }));
    } finally {
      setBusy("");
    }
  }

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
    <main className={styles.console}>
      <aside className={styles.sidebar}>
        <div className={styles.brand}><span className={styles.brandMark}>C</span><div><strong>CodeLocal</strong><small>POOL</small></div></div>
        <nav className={styles.nav}>
          <a className={styles.navActive} href="#overview"><span>◫</span>Overview</a>
          <a href="#models"><span>◇</span>Models</a>
          <a href="#sources"><span>◆</span>Sources</a>
          <a href="#models"><span>▶</span>Model Test</a>
        </nav>
        <div className={styles.sidebarFoot}><span className={styles.statusDot} />9Router control plane</div>
      </aside>

      <section className={styles.workspace}>
        <header className={styles.topbar}>
          <div><span className="eyebrow">Routing control plane</span><h1>Pool</h1><p>Canonical models for CodeLocal · actual 9Router quota and connection state</p></div>
          <div className={styles.actions}><button onClick={() => void refresh()} type="button">↻ Refresh</button><button onClick={() => void logout()} type="button">Logout</button></div>
        </header>

        <div className={styles.content}>
          <section id="overview" className={styles.metrics}>
            <article><span>Usable models</span><strong>{active.length}</strong><small>{snapshot.models.length} discovered</small></article>
            <article><span>Degraded</span><strong>{degraded.length}</strong><small>still routable</small></article>
            <article><span>Unavailable</span><strong>{unavailable.length}</strong><small>hidden from /v1/models</small></article>
            <article><span>Routes</span><strong>{availableRoutes}<i>/{totalRoutes}</i></strong><small>available / total</small></article>
          </section>

          {error ? <div className={styles.alert}>{error}</div> : null}
          {loading ? <div className={styles.loading}>Đang đọc trạng thái thực từ 9Router…</div> : null}

          <section id="models" className={styles.panel}>
            <header className={styles.panelHeader}>
              <div><span className="eyebrow">CodeLocal contract</span><h2>Models</h2><p>Chỉ model còn ít nhất một route usable mới xuất hiện ở public <code>/v1/models</code>.</p></div>
              <label className={styles.search}><span>⌕</span><input onChange={(event) => setQuery(event.target.value)} placeholder="Search model or provider" value={query} /></label>
            </header>
            <div className={styles.tableWrap}>
              <div className={styles.tableHead}><span>Model</span><span>Provider / upstream</span><span>Routes</span><span>Quota left</span><span>Reset / retry</span><span>Status</span><span /></div>
              <div className={styles.modelList}>
                {visibleModels.map((model) => {
                  const route = primaryRoute(model);
                  const result = tests[model.id];
                  return (
                    <div className={styles.modelRow} data-state={model.state} key={model.id}>
                      <div className={styles.modelIdentity}><span className={styles.dot} data-active={model.active} /><div><code>{model.id}</code><small>{model.availableSources}/{model.totalSources} Pool source</small></div></div>
                      <div className={styles.routeCell}><strong>{route?.provider || route?.source || "—"}</strong><code title={route?.upstream || ""}>{route?.upstream || "—"}</code></div>
                      <div className={styles.routeCount}><strong>{model.availableRoutes ?? model.availableSources}</strong><span>/ {model.totalRoutes ?? model.totalSources}</span></div>
                      <div className={styles.quota}><strong>{quotaLabel(route)}</strong>{typeof route?.quotaRemainingPercent === "number" ? <span><i style={{ width: `${Math.max(0, Math.min(100, route.quotaRemainingPercent))}%` }} /></span> : null}</div>
                      <small className={styles.reset}>{resetLabel(route)}</small>
                      <span className={styles.stateBadge} data-state={model.active ? model.state : model.state || "unavailable"}>{modelLabel(model)}</span>
                      <button className={styles.testButton} disabled={busy !== "" || !model.active} onClick={() => void testModel(model)} type="button">{busy === `test:${model.id}` ? "Testing…" : "Test"}</button>
                      {result ? <div className={styles.testResult} data-ok={result.ok}><strong>{result.ok ? `✓ ${result.httpStatus || 200}` : "✕ Failed"}</strong><span>{result.ok ? `${result.provider || result.source || "route"} · ${result.latencyMs ?? 0} ms · ${result.attempts} attempt${result.attempts === 1 ? "" : "s"}` : result.error || "Unknown error"}</span></div> : null}
                    </div>
                  );
                })}
                {!visibleModels.length && !loading ? <p className={styles.empty}>Không có model phù hợp.</p> : null}
              </div>
            </div>
          </section>

          <section id="sources" className={styles.panel}>
            <header className={styles.panelHeader}><div><span className="eyebrow">Execution layer</span><h2>Sources</h2><p>Phase 1 để 9Router quản lý account/provider; Pool chỉ tạo contract canonical cho CodeLocal.</p></div></header>
            <div className={styles.sourceList}>
              {snapshot.sources.map((source) => (
                <div className={styles.sourceRow} key={source.id}>
                  <div className={styles.sourceIcon}>9R</div>
                  <div className={styles.identity}><strong>{source.name}</strong><small>{source.kind} · priority {source.priority}</small><code>{source.baseUrl}</code></div>
                  <span className={styles.badge} data-enabled={source.enabled}>{source.enabled ? "Connected" : "Disabled"}</span>
                  {source.managedBy === "pool" ? <div className={styles.rowActions}><button disabled={busy !== ""} onClick={() => void setSourceEnabled(source, !source.enabled)}>{source.enabled ? "Disable" : "Enable"}</button><button disabled={busy !== ""} onClick={() => void removeSource(source)}>Delete</button></div> : <small className={styles.managed}>Environment managed</small>}
                </div>
              ))}
            </div>
          </section>

          {snapshot.storageConfigured ? <section className={styles.panel}>
            <header className={styles.panelHeader}><div><span className="eyebrow">Future source lane</span><h2>Add OpenAI-compatible source</h2><p>Giữ sẵn cho Phase 2; credential được mã hoá AES-GCM trong Pool API.</p></div></header>
            <form className={styles.form} onSubmit={addSource}>
              <label><span>Name</span><input maxLength={120} onChange={(event) => setName(event.target.value)} required value={name} /></label>
              <label><span>Base URL</span><input onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://api.example.com/v1" required type="url" value={baseUrl} /></label>
              <label><span>Credential</span><input autoComplete="off" onChange={(event) => setApiKey(event.target.value)} required type="password" value={apiKey} /></label>
              <label><span>Priority</span><input min="0" max="10000" onChange={(event) => setPriority(event.target.value)} required type="number" value={priority} /></label>
              <button disabled={busy !== ""} type="submit">{busy === "create" ? "Validating…" : "Validate & add"}</button>
            </form>
          </section> : null}

          <footer className={styles.footer}><span>CodeLocal → <code>pool.codelocal.cloud/v1</code></span><span>Pool → canonical active catalog</span><span>9Router → accounts · quota · provider execution</span></footer>
        </div>
      </section>
    </main>
  );
}
