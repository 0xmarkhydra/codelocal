"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { LanguageSelect, useTranslations } from "@codelocal/i18n/provider";
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
  const data = await response.json().catch(() => null) as Record<string, unknown> | null;
  if (!data || typeof data !== "object" || Array.isArray(data)) throw new Error("Invalid Pool response.");
  const codes: Record<string, string> = {
    pool_unavailable: "Pool unavailable",
    invalid_pool_response: "Invalid Pool response.",
    unauthorized: "Session expired. Sign in again.",
    invalid_origin: "Request origin did not match. Reload and try again.",
    invalid_request: "Invalid request.",
    invalid_source: "Invalid source details.",
    invalid_source_update: "Invalid source update.",
    invalid_model: "Invalid model ID.",
  };
  if (!response.ok && typeof data.error === "string" && Object.hasOwn(codes, data.error)) throw new Error(codes[data.error]);
  if (!response.ok) throw new Error(typeof data.detail === "string" ? data.detail : typeof data.error === "string" ? data.error : "Pool request failed.");
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

function quotaLabel(locale: string, source?: ModelSource) {
  if (typeof source?.quotaRemainingPercent !== "number") return "—";
  return new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 }).format(Math.max(0, Math.min(100, source.quotaRemainingPercent)) / 100);
}

function resetLabel(locale: string, source?: ModelSource) {
  const raw = source?.quotaResetAt || source?.retryAt;
  if (!raw) return "—";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString(locale, { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

function primaryRoute(model: Model) {
  return model.sources?.find((source) => source.state === "healthy") || model.sources?.find((source) => source.state === "degraded") || model.sources?.[0];
}

export function PoolDashboard() {
  const router = useRouter();
  const { locale, t, message } = useTranslations();
  const number = (value: number) => new Intl.NumberFormat(locale).format(value);
  const [snapshot, setSnapshot] = useState<Snapshot>({ models: [], sources: [], storageConfigured: false });
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [tests, setTests] = useState<Record<string, TestResult>>({});
  const [name, setName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [priority, setPriority] = useState("50");

  const refresh = useCallback(async () => {
    setLoading(true);
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
      if (!Array.isArray(modelsData.models) || !Array.isArray(sourcesData.sources)) throw new Error("Invalid Pool response.");
      setSnapshot({
        models: Array.isArray(modelsData.models) ? modelsData.models : [],
        sources: Array.isArray(sourcesData.sources) ? sourcesData.sources : [],
        storageConfigured: sourcesData.storageConfigured === true,
      });
      setLoaded(true);
    } catch (cause) {
      setError(cause instanceof Error && !(cause instanceof TypeError) ? cause.message : "Pool unavailable");
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
  const totalRoutes = useMemo(() => snapshot.models.reduce((sum, model) => sum + (model.totalRoutes ?? model.totalSources ?? 0), 0), [snapshot.models]);
  const availableRoutes = useMemo(() => snapshot.models.reduce((sum, model) => sum + (model.availableRoutes ?? model.availableSources ?? 0), 0), [snapshot.models]);
  const visibleModels = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return snapshot.models;
    return snapshot.models.filter((model) => model.id.toLowerCase().includes(needle) || model.sources?.some((source) => `${source.provider || ""} ${source.upstream || ""}`.toLowerCase().includes(needle)));
  }, [query, snapshot.models]);

  async function testModel(model: Model) {
    setBusy(`test:${model.id}`);
    setError("");
    try {
      const response = await fetch(`/api/pool/models/${encodeURIComponent(model.id)}/test`, { method: "POST" });
      const result = await readJSON(response) as unknown as TestResult;
      setTests((current) => ({ ...current, [model.id]: result }));
      await refresh();
    } catch (cause) {
      setTests((current) => ({ ...current, [model.id]: { model: model.id, ok: false, attempts: 0, error: cause instanceof Error && !(cause instanceof TypeError) ? cause.message : "Test failed" } }));
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
        body: JSON.stringify({ name, kind: "openai-compatible", baseUrl, apiKey, priority: Number(priority) }),
      });
      await readJSON(response);
      setName(""); setBaseUrl(""); setApiKey(""); setPriority("50");
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error && !(cause instanceof TypeError) ? cause.message : "Could not add source");
    } finally { setBusy(""); }
  }

  async function setSourceEnabled(source: Source, enabled: boolean) {
    setBusy(`toggle:${source.id}`); setError("");
    try {
      await readJSON(await fetch(`/api/pool/sources/${encodeURIComponent(source.id)}`, {
        method: "PATCH", headers: { "content-type": "application/json" }, body: JSON.stringify({ enabled }),
      }));
      await refresh();
    } catch (cause) { setError(cause instanceof Error && !(cause instanceof TypeError) ? cause.message : "Could not update source"); }
    finally { setBusy(""); }
  }

  async function removeSource(source: Source) {
    if (!window.confirm(t("Delete source “{name}”? This cannot be undone.", { name: source.name }))) return;
    setBusy(`delete:${source.id}`); setError("");
    try {
      await readJSON(await fetch(`/api/pool/sources/${encodeURIComponent(source.id)}`, { method: "DELETE" }));
      await refresh();
    } catch (cause) { setError(cause instanceof Error && !(cause instanceof TypeError) ? cause.message : "Could not delete source"); }
    finally { setBusy(""); }
  }

  async function logout() {
    setBusy("logout"); setError("");
    try {
      await readJSON(await fetch("/api/session/logout", { method: "POST" }));
      router.replace("/login");
      router.refresh();
    } catch {
      setError("Could not sign out. Try again.");
    } finally { setBusy(""); }
  }

  return (
    <main className={styles.console}>
      <aside className={styles.sidebar}>
        <div className={styles.brand}><span className={styles.brandMark}>C</span><div><strong>CodeLocal</strong><small>POOL</small></div></div>
        <nav className={styles.nav} aria-label={t("Pool navigation")}>
          <a href="#overview">{t("Overview")}</a>
          <a href="#models">{t("Models")}</a>
          <a href="#sources">{t("Sources")}</a>
        </nav>
        <div className={styles.sidebarFoot}>9Router</div>
      </aside>

      <section className={styles.workspace}>
        <header className={styles.topbar}>
          <h1>Pool</h1>
          <div className={styles.actions}><LanguageSelect /><button disabled={loading || busy !== ""} onClick={() => void refresh()} type="button">{t("Refresh")}</button><button disabled={busy !== ""} onClick={() => void logout()} type="button">{t("Sign out")}</button></div>
        </header>

        <div className={styles.content}>
          <section id="overview" className={styles.metrics} aria-busy={loading}>
            <article><span>{t("Usable models")}</span><strong>{loaded ? number(active.length) : "—"}</strong><small>{loaded ? t("{count} discovered", { count: snapshot.models.length }) : t("Loading…")}</small></article>
            <article><span>{t("Degraded")}</span><strong>{loaded ? number(degraded.length) : "—"}</strong><small>{t("Still routable")}</small></article>
            <article><span>{t("Unavailable")}</span><strong>{loaded ? number(unavailable.length) : "—"}</strong><small>{t("Hidden from {path}", { path: "/v1/models" })}</small></article>
            <article><span>{t("Routes")}</span><strong>{loaded ? number(availableRoutes) : "—"}<i>/{loaded ? number(totalRoutes) : "—"}</i></strong><small>{t("Available / total")}</small></article>
          </section>

          {error ? <div className={styles.alert} role="alert">{message(error)}</div> : null}
          {loading ? <div className={styles.loading} role="status">{t("Loading Pool status…")}</div> : null}

          <section id="models" className={styles.panel}>
            <header className={styles.panelHeader}>
              <h2>{t("Models")}</h2>
              <label className={styles.search}><input aria-label={t("Search model or provider")} onChange={(event) => setQuery(event.target.value)} placeholder={t("Search model or provider")} value={query} /></label>
            </header>
            <div className={styles.tableWrap}>
              <div className={styles.tableHead}><span>{t("Model")}</span><span>{t("Provider / upstream")}</span><span>{t("Routes")}</span><span>{t("Quota left")}</span><span>{t("Reset / retry")}</span><span>{t("Status")}</span><span /></div>
              <div className={styles.modelList}>
                {visibleModels.map((model) => {
                  const route = primaryRoute(model);
                  const result = tests[model.id];
                  return (
                    <div className={styles.modelRow} data-state={model.state} key={model.id}>
                      <div className={styles.modelIdentity}><span className={styles.dot} data-active={model.active} /><div><code>{model.id}</code><small>{t("{available}/{total} Pool sources", { available: model.availableSources, total: model.totalSources })}</small></div></div>
                      <div className={styles.routeCell}><strong>{route?.provider || route?.source || "—"}</strong><code title={route?.upstream || ""}>{route?.upstream || "—"}</code></div>
                      <div className={styles.routeCount}><strong>{number(model.availableRoutes ?? model.availableSources)}</strong><span>/ {number(model.totalRoutes ?? model.totalSources)}</span></div>
                      <div className={styles.quota}><strong>{quotaLabel(locale, route)}</strong>{typeof route?.quotaRemainingPercent === "number" ? <span><i style={{ width: `${Math.max(0, Math.min(100, route.quotaRemainingPercent))}%` }} /></span> : null}</div>
                      <small className={styles.reset}>{resetLabel(locale, route)}</small>
                      <span className={styles.stateBadge} data-state={model.active ? model.state : model.state || "unavailable"}>{message(modelLabel(model))}</span>
                      <button className={styles.testButton} aria-label={t("Test {model}", { model: model.id })} disabled={busy !== "" || !model.active} onClick={() => void testModel(model)} type="button">{busy === `test:${model.id}` ? t("Testing…") : t("Test")}</button>
                      {result ? <div className={styles.testResult} data-ok={result.ok} role="status"><strong>{result.ok ? `HTTP ${result.httpStatus ?? 200}` : t("Failed")}</strong><span>{result.ok ? `${result.provider || result.source || t("Route")} · ${result.latencyMs == null ? "—" : number(result.latencyMs)} ms · ${t("{count} attempts", { count: result.attempts })}` : message(result.error || "Unknown error")}</span></div> : null}
                    </div>
                  );
                })}
                {!visibleModels.length && !loading && loaded ? <p className={styles.empty}>{t("No matching models.")}</p> : null}
              </div>
            </div>
          </section>

          <section id="sources" className={styles.panel}>
            <header className={styles.panelHeader}><h2>{t("Sources")}</h2></header>
            <div className={styles.sourceList}>
              {snapshot.sources.map((source) => (
                <div className={styles.sourceRow} key={source.id}>
                  <div className={styles.sourceIcon} aria-hidden="true">API</div>
                  <div className={styles.identity}><strong>{source.name}</strong><small>{source.kind} · {t("Priority {priority}", { priority: source.priority })}</small><code>{source.baseUrl}</code></div>
                  <span className={styles.badge} data-enabled={source.enabled}>{source.enabled ? t("Enabled") : t("Disabled")}</span>
                  {source.managedBy === "pool" ? <div className={styles.rowActions}><label><input type="checkbox" disabled={busy !== ""} checked={source.enabled} onChange={(event) => void setSourceEnabled(source, event.target.checked)} /><span className="sr-only">{t("Enable source {name}", { name: source.name })}</span></label><button disabled={busy !== ""} onClick={() => void removeSource(source)}>{t("Delete")}</button></div> : <small className={styles.managed}>{t("Environment managed")}</small>}
                </div>
              ))}
              {!snapshot.sources.length && !loading && loaded ? <p className={styles.empty}>{t("No sources configured.")}</p> : null}
            </div>
          </section>

          {snapshot.storageConfigured ? <section className={styles.panel}>
            <header className={styles.panelHeader}><h2>{t("Add OpenAI-compatible source")}</h2></header>
            <form className={styles.form} onSubmit={addSource}>
              <label><span>{t("Name")}</span><input maxLength={120} onChange={(event) => setName(event.target.value)} required value={name} /></label>
              <label><span>{t("Base URL")}</span><input onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://api.example.com/v1" required type="url" value={baseUrl} /></label>
              <label><span>{t("Credential")}</span><input autoComplete="off" onChange={(event) => setApiKey(event.target.value)} required type="password" value={apiKey} /></label>
              <label><span>{t("Priority")}</span><input min="0" max="10000" step="1" onChange={(event) => setPriority(event.target.value)} required type="number" value={priority} /></label>
              <button disabled={busy !== ""} type="submit">{busy === "create" ? t("Validating…") : t("Validate & add")}</button>
            </form>
          </section> : null}

          <footer className={styles.footer}><span>CodeLocal Pool</span><code>pool.codelocal.cloud/v1</code></footer>
        </div>
      </section>
    </main>
  );
}
