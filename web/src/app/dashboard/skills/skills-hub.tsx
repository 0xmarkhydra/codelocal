"use client";

import { ChangeEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AppIcon } from "../app-icon";
import styles from "./skills.module.css";

type SkillView = "for-you" | "explore" | "mine" | "built-in";
type SkillMode = "auto" | "prefer" | "disabled";

type SkillItem = {
  id: string;
  name: string;
  version: string;
  publisher: string;
  scope: "system" | "personal" | "community" | string;
  kind: "knowledge" | "workflow" | "runtime" | "hybrid" | string;
  state: string;
  tags?: string[];
  capabilities?: string[];
  quality: number;
  verified: boolean;
  license?: string;
  mode: SkillMode | string;
  pinnedVersion?: string;
  userRating?: number;
  ratingAverage?: number;
  ratingCount?: number;
};

type SkillsResource = {
  autoUse: boolean;
  storageConfigured: boolean;
  items: SkillItem[];
};

type AccountResource = {
  csrf: string;
  isAdmin: boolean;
  requiresReauthentication: boolean;
};

type AdminSkillItem = {
  id: string;
  name: string;
  version: string;
  scope: string;
  kind: string;
  publisher: string;
  state: string;
  quality: number;
  verified: boolean;
  packageHash: string;
};

type AdminResource = { items?: AdminSkillItem[] };

type Notice = { kind: "success" | "error"; text: string } | null;

const views: Array<{ id: SkillView; label: string }> = [
  { id: "for-you", label: "For You" },
  { id: "explore", label: "Explore" },
  { id: "mine", label: "My Skills" },
  { id: "built-in", label: "Built-in" },
];

function scopeLabel(scope: string) {
  switch (scope) {
    case "system": return "Built-in";
    case "personal": return "Personal";
    case "community": return "Community";
    default: return scope;
  }
}

function permissionLabel(skill: SkillItem) {
  const capabilities = skill.capabilities?.filter(Boolean) ?? [];
  if (skill.kind === "knowledge" && capabilities.length === 0) return "Knowledge only · no runtime permissions";
  if (capabilities.length === 0) return `${skill.kind} · authorization required at execution`;
  return `Requires ${capabilities.join(", ")}`;
}

function stateLabel(state: string) {
  return state.replaceAll("_", " ");
}

function isRoutableState(skill: SkillItem) {
  return skill.state === "active" || skill.state === "promoted";
}

async function responseError(response: Response) {
  try {
    const payload = await response.json() as { detail?: string; error?: string };
    return payload.detail || payload.error || `Request failed (${response.status})`;
  } catch {
    return `Request failed (${response.status})`;
  }
}

function SkillCard({
  skill,
  busy,
  mutateState,
  rate,
}: {
  skill: SkillItem;
  busy: boolean;
  mutateState: (skill: SkillItem, mode: SkillMode, pinnedVersion?: string) => Promise<void>;
  rate: (skill: SkillItem, rating: number) => Promise<void>;
}) {
  const tags = (skill.tags ?? []).slice(0, 4);
  const mode = (skill.mode || "auto") as SkillMode;
  const canRate = skill.scope === "community" && skill.state === "promoted";
  return (
    <article className={styles.skillCard}>
      <div className={styles.skillIcon} aria-hidden="true">
        <AppIcon name="skill" size={21} />
      </div>
      <div className={styles.skillBody}>
        <div className={styles.skillHeading}>
          <div>
            <h2>{skill.name}</h2>
            <p>{skill.publisher} · {scopeLabel(skill.scope)} · v{skill.version}</p>
          </div>
          <span className={styles.autoBadge}>{stateLabel(skill.state)}</span>
        </div>
        <p className={styles.description}>
          {tags.length ? tags.join(" · ") : "Reusable CodeLocal expertise selected automatically when it materially improves the task."}
        </p>
        {tags.length > 0 && (
          <div className={styles.tags} aria-label={`${skill.name} tags`}>
            {tags.map((tag) => <span key={tag}>{tag}</span>)}
          </div>
        )}
        <div className={styles.skillMeta}>
          <span>Quality {Math.round(skill.quality * 100)}%</span>
          {skill.ratingCount ? <span>★ {skill.ratingAverage?.toFixed(1)} · {skill.ratingCount}</span> : null}
          {skill.pinnedVersion ? <span>Pinned v{skill.pinnedVersion}</span> : null}
        </div>
        <div className={styles.permissionLine}>
          <AppIcon name="shield" size={15} />
          <span>{permissionLabel(skill)}</span>
        </div>
        <div className={styles.skillControls} aria-label={`${skill.name} behavior`}>
          {(["auto", "prefer", "disabled"] as SkillMode[]).map((choice) => (
            <button
              className={mode === choice ? styles.controlActive : undefined}
              disabled={busy}
              key={choice}
              onClick={() => void mutateState(skill, choice, skill.pinnedVersion)}
              type="button"
            >
              {choice === "auto" ? "Auto" : choice === "prefer" ? "Prefer" : "Disable"}
            </button>
          ))}
          {isRoutableState(skill) && (
            <button
              disabled={busy}
              onClick={() => void mutateState(skill, mode, skill.pinnedVersion ? "" : skill.version)}
              type="button"
            >
              {skill.pinnedVersion ? "Unpin" : "Pin version"}
            </button>
          )}
        </div>
        {canRate && (
          <div className={styles.rating} aria-label={`Rate ${skill.name}`}>
            <span>Your rating</span>
            {[1, 2, 3, 4, 5].map((value) => (
              <button
                aria-label={`${value} star${value === 1 ? "" : "s"}`}
                className={(skill.userRating ?? 0) >= value ? styles.ratingActive : undefined}
                disabled={busy}
                key={value}
                onClick={() => void rate(skill, value)}
                type="button"
              >★</button>
            ))}
          </div>
        )}
      </div>
    </article>
  );
}

function EmptyView({ title, copy }: { title: string; copy: string }) {
  return (
    <div className={styles.emptyState}>
      <AppIcon name="skill" size={22} />
      <strong>{title}</strong>
      <span>{copy}</span>
    </div>
  );
}

function AdminPanel({
  items,
  busy,
  action,
  importAdmin,
  storageConfigured,
}: {
  items: AdminSkillItem[];
  busy: boolean;
  action: (item: AdminSkillItem, action: "evaluate-start" | "evaluate" | "promote" | "rollback", score?: number, evidence?: string) => Promise<void>;
  importAdmin: () => void;
  storageConfigured: boolean;
}) {
  const [scores, setScores] = useState<Record<string, string>>({});
  const [evidence, setEvidence] = useState<Record<string, string>>({});
  return (
    <section className={styles.adminPanel}>
      <div className={styles.adminHeader}>
        <div>
          <p className={styles.eyebrow}>Admin moderation</p>
          <h2>Skill promotion queue</h2>
        </div>
        <button className={styles.primaryButton} disabled={busy || !storageConfigured} onClick={importAdmin} type="button">Import candidate</button>
      </div>
      {items.length === 0 ? <p className={styles.muted}>No shared Skill versions are waiting for moderation.</p> : items.map((item) => {
        const key = `${item.id}@${item.version}`;
        return (
          <div className={styles.adminRow} key={key}>
            <div>
              <strong>{item.name} · v{item.version}</strong>
              <span>{item.publisher} · {item.scope} · {stateLabel(item.state)}</span>
            </div>
            <div className={styles.adminActions}>
              {item.state === "candidate" && <button disabled={busy} onClick={() => void action(item, "evaluate-start")} type="button">Start evaluation</button>}
              {item.state === "evaluating" && (
                <>
                  <input
                    aria-label={`Evaluation score for ${item.name}`}
                    max="1"
                    min="0"
                    onChange={(event) => setScores((current) => ({ ...current, [key]: event.target.value }))}
                    placeholder="0.90"
                    step="0.01"
                    type="number"
                    value={scores[key] ?? ""}
                  />
                  <input
                    aria-label={`Evaluation evidence for ${item.name}`}
                    onChange={(event) => setEvidence((current) => ({ ...current, [key]: event.target.value }))}
                    placeholder="Evidence summary"
                    type="text"
                    value={evidence[key] ?? ""}
                  />
                  <button
                    disabled={busy || !scores[key] || !evidence[key]}
                    onClick={() => void action(item, "evaluate", Number(scores[key]), evidence[key])}
                    type="button"
                  >Submit evaluation</button>
                </>
              )}
              {item.state === "canary" && <button disabled={busy} onClick={() => void action(item, "promote")} type="button">Promote stable</button>}
              {item.state === "promoted" && <button disabled={busy} onClick={() => void action(item, "rollback")} type="button">Rollback</button>}
            </div>
          </div>
        );
      })}
    </section>
  );
}

export function SkillsHub() {
  const [view, setView] = useState<SkillView>("for-you");
  const [resource, setResource] = useState<SkillsResource>({ autoUse: true, storageConfigured: false, items: [] });
  const [account, setAccount] = useState<AccountResource | null>(null);
  const [adminItems, setAdminItems] = useState<AdminSkillItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);
  const personalInput = useRef<HTMLInputElement>(null);
  const communityInput = useRef<HTMLInputElement>(null);
  const adminInput = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    const [skillsResponse, accountResponse] = await Promise.all([
      fetch("/api/v1/skills", { credentials: "include", cache: "no-store" }),
      fetch("/api/v1/account", { credentials: "include", cache: "no-store" }),
    ]);
    if (!skillsResponse.ok) throw new Error(await responseError(skillsResponse));
    if (!accountResponse.ok) throw new Error(await responseError(accountResponse));
    const skills = await skillsResponse.json() as SkillsResource;
    const nextAccount = await accountResponse.json() as AccountResource;
    setResource({ autoUse: skills.autoUse !== false, storageConfigured: skills.storageConfigured === true, items: Array.isArray(skills.items) ? skills.items : [] });
    setAccount(nextAccount);
    if (nextAccount.isAdmin) {
      const adminResponse = await fetch("/api/v1/admin/skills", { credentials: "include", cache: "no-store" });
      if (adminResponse.ok) {
        const admin = await adminResponse.json() as AdminResource;
        setAdminItems(Array.isArray(admin.items) ? admin.items : []);
      }
    } else {
      setAdminItems([]);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    void load()
      .then(() => { if (!cancelled) setLoadFailed(false); })
      .catch(() => { if (!cancelled) setLoadFailed(true); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [load]);

  const refresh = useCallback(async () => {
    try {
      await load();
      setLoadFailed(false);
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Could not refresh Skills." });
    }
  }, [load]);

  const mutation = useCallback(async (url: string, method: string, body?: unknown) => {
    if (!account?.csrf) throw new Error("Security token unavailable. Refresh the page and try again.");
    const response = await fetch(url, {
      method,
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": account.csrf },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) throw new Error(await responseError(response));
    return response;
  }, [account?.csrf]);

  const withMutation = useCallback(async (work: () => Promise<void>, success: string) => {
    setBusy(true);
    setNotice(null);
    try {
      await work();
      await refresh();
      setNotice({ kind: "success", text: success });
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Skill operation failed." });
    } finally {
      setBusy(false);
    }
  }, [refresh]);

  const uploadPackage = useCallback(async (event: ChangeEvent<HTMLInputElement>, endpoint: string, success: string) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    await withMutation(async () => {
      let payload: unknown;
      try { payload = JSON.parse(await file.text()); } catch { throw new Error("The selected file is not valid .skill.json JSON."); }
      await mutation(endpoint, "POST", payload);
    }, success);
  }, [mutation, withMutation]);

  const mutateState = useCallback(async (skill: SkillItem, mode: SkillMode, pinnedVersion = "") => {
    await withMutation(
      async () => { await mutation(`/api/v1/skills/${encodeURIComponent(skill.id)}/state`, "PATCH", { mode, pinnedVersion }); },
      `${skill.name} behavior updated.`,
    );
  }, [mutation, withMutation]);

  const rate = useCallback(async (skill: SkillItem, rating: number) => {
    await withMutation(
      async () => { await mutation(`/api/v1/skills/${encodeURIComponent(skill.id)}/rating`, "POST", { rating }); },
      `Rated ${skill.name} ${rating}/5.`,
    );
  }, [mutation, withMutation]);

  const adminAction = useCallback(async (item: AdminSkillItem, action: "evaluate-start" | "evaluate" | "promote" | "rollback", score?: number, evidence?: string) => {
    const base = `/api/v1/admin/skills/${encodeURIComponent(item.id)}/versions/${encodeURIComponent(item.version)}`;
    const suffix = action === "evaluate-start" ? "/evaluate/start" : action === "evaluate" ? "/evaluate" : `/${action}`;
    await withMutation(async () => {
      await mutation(`${base}${suffix}`, "POST", action === "evaluate" ? { score, evidence } : undefined);
    }, `${item.name} ${action.replace("-", " ")} completed.`);
  }, [mutation, withMutation]);

  const visibleSkills = useMemo(() => {
    switch (view) {
      case "built-in": return resource.items.filter((skill) => skill.scope === "system");
      case "explore": return resource.items.filter((skill) => skill.scope === "community");
      case "mine": return resource.items.filter((skill) => skill.scope === "personal");
      default: return resource.items.filter((skill) => skill.mode !== "disabled" && isRoutableState(skill));
    }
  }, [resource.items, view]);

  const emptyCopy = view === "explore"
    ? { title: "No Community Skills yet", copy: "Published candidates and promoted Community Skills will appear here. Candidates are never auto-routed before evaluation and promotion." }
    : view === "mine"
      ? { title: "No Personal Skills yet", copy: "Import a private .skill.json package. It stays scoped to your account and can be reused across projects." }
      : { title: "No Skills available yet", copy: "CodeLocal will surface reusable expertise here when the authoritative catalog contains a match." };

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>CodeLocal Intelligence</p>
          <h1>Skills</h1>
          <p className={styles.lede}>Chat normally. CodeLocal finds and combines useful expertise automatically.</p>
        </div>
        <div className={styles.status} data-active={resource.autoUse || undefined}>
          <span className={styles.statusDot} aria-hidden="true" />
          {resource.autoUse ? "Auto-use on" : "Auto-use off"}
        </div>
      </header>

      <div className={styles.toolbar}>
        <div>
          <button className={styles.primaryButton} disabled={busy || !resource.storageConfigured} onClick={() => personalInput.current?.click()} type="button">Import Personal</button>
          <button disabled={busy || !resource.storageConfigured} onClick={() => communityInput.current?.click()} type="button">Publish Community</button>
        </div>
        <span>{resource.storageConfigured ? "Cloud Skill storage ready" : "Cloud Skill storage is not configured"}</span>
        <input accept=".json,.skill.json,application/json" hidden onChange={(event) => void uploadPackage(event, "/api/v1/skills/import", "Personal Skill imported.")} ref={personalInput} type="file" />
        <input accept=".json,.skill.json,application/json" hidden onChange={(event) => void uploadPackage(event, "/api/v1/skills/publish", "Community Skill submitted for review.")} ref={communityInput} type="file" />
        <input accept=".json,.skill.json,application/json" hidden onChange={(event) => void uploadPackage(event, "/api/v1/admin/skills/import", "Admin Skill candidate imported.")} ref={adminInput} type="file" />
      </div>

      {account?.requiresReauthentication && <div className={styles.noticeError}>Sensitive Skill imports require a fresh sign-in session.</div>}
      {notice && <div className={notice.kind === "error" ? styles.noticeError : styles.noticeSuccess}>{notice.text}</div>}

      <nav className={styles.tabs} aria-label="Skill views">
        {views.map((item) => (
          <button aria-current={view === item.id ? "page" : undefined} className={view === item.id ? styles.activeTab : undefined} key={item.id} onClick={() => setView(item.id)} type="button">{item.label}</button>
        ))}
      </nav>

      <div className={styles.content}>
        <div className={styles.sectionIntro}>
          <h2>{view === "for-you" ? "Ready when relevant" : view === "built-in" ? "Built-in intelligence" : view === "explore" ? "Community market" : "Your reusable skills"}</h2>
          <p>{view === "for-you" ? "No install step. Eligible Skills are selected directly from chat." : "Manage visibility and preference without mixing project-specific knowledge into Project Brain."}</p>
        </div>
        {loading && <EmptyView title="Loading Skills" copy="Reading your current CodeLocal Skill catalog…" />}
        {!loading && loadFailed && <EmptyView title="Skills unavailable" copy="The Skill API could not be loaded. Chat continues with safe built-in fallback knowledge." />}
        {!loading && !loadFailed && visibleSkills.map((skill) => <SkillCard busy={busy} key={`${skill.scope}:${skill.id}@${skill.version}`} mutateState={mutateState} rate={rate} skill={skill} />)}
        {!loading && !loadFailed && visibleSkills.length === 0 && <EmptyView title={emptyCopy.title} copy={emptyCopy.copy} />}
      </div>

      {account?.isAdmin && (
        <AdminPanel action={adminAction} busy={busy} importAdmin={() => adminInput.current?.click()} items={adminItems} storageConfigured={resource.storageConfigured} />
      )}
    </section>
  );
}
