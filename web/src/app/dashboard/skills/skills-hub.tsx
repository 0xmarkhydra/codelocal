"use client";

import { ChangeEvent, DragEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AppIcon } from "../app-icon";
import styles from "./skills.module.css";
import smart from "./skills-smart-add.module.css";

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
type SourceDocument = { path: string; content: string };
type Notice = { kind: "success" | "error"; text: string } | null;

const views: Array<{ id: SkillView; label: string }> = [
  { id: "for-you", label: "For You" },
  { id: "mine", label: "My Skills" },
  { id: "explore", label: "Explore" },
  { id: "built-in", label: "Built-in" },
];

const knowledgeFilePattern = /\.(md|txt|json|ya?ml|csv)$/i;

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

function looksLikeSkillPackage(payload: unknown) {
  if (!payload || typeof payload !== "object") return false;
  const candidate = payload as Record<string, unknown>;
  return typeof candidate.formatVersion === "number"
    && typeof candidate.manifest === "object"
    && typeof candidate.artifact === "object"
    && typeof candidate.packageHash === "string";
}

async function responseError(response: Response) {
  try {
    const payload = await response.json() as { detail?: string; error?: string };
    return payload.detail || payload.error || `Request failed (${response.status})`;
  } catch {
    return `Request failed (${response.status})`;
  }
}

async function fileToBase64(file: File) {
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error(`Could not read ${file.name}.`));
    reader.onload = () => {
      const result = String(reader.result ?? "");
      const marker = result.indexOf(",");
      resolve(marker >= 0 ? result.slice(marker + 1) : result);
    };
    reader.readAsDataURL(file);
  });
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
  const [dragging, setDragging] = useState(false);
  const [draft, setDraft] = useState("");
  const [notice, setNotice] = useState<Notice>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const folderInput = useRef<HTMLInputElement>(null);
  const communityInput = useRef<HTMLInputElement>(null);
  const adminInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    folderInput.current?.setAttribute("webkitdirectory", "");
    folderInput.current?.setAttribute("directory", "");
  }, []);

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
      return true;
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Skill operation failed." });
      return false;
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

  const ingestPayload = useCallback(async (payload: Record<string, unknown>, success: string) => {
    const completed = await withMutation(async () => {
      await mutation("/api/v1/skills/ingest", "POST", payload);
    }, success);
    if (completed) setView("mine");
    return completed;
  }, [mutation, withMutation]);

  const submitDraft = useCallback(async () => {
    const value = draft.trim();
    if (!value) return;
    const payload = /^https?:\/\//i.test(value) ? { sourceUrl: value } : { text: value };
    if (await ingestPayload(payload, "Personal Skill added and ready for Auto routing.")) {
      setDraft("");
    }
  }, [draft, ingestPayload]);

  const ingestFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return;
    if (files.length === 1) {
      const file = files[0];
      const lower = file.name.toLowerCase();
      if (lower.endsWith(".zip")) {
        await ingestPayload({ archiveBase64: await fileToBase64(file), name: file.name.replace(/\.zip$/i, "") }, `${file.name} added as a Personal Skill.`);
        return;
      }
      if (lower.endsWith(".skill.json")) {
        let payload: unknown;
        try { payload = JSON.parse(await file.text()); } catch { throw new Error(`${file.name} is not valid JSON.`); }
        await ingestPayload({ package: payload }, `${file.name} imported as a Personal Skill.`);
        return;
      }
      if (lower.endsWith(".json")) {
        try {
          const payload = JSON.parse(await file.text()) as unknown;
          if (looksLikeSkillPackage(payload)) {
            await ingestPayload({ package: payload }, `${file.name} imported as a Personal Skill.`);
            return;
          }
        } catch {
          // A regular JSON knowledge document is handled below and validated by the server.
        }
      }
    }

    const documents: SourceDocument[] = [];
    const unsupported: string[] = [];
    for (const file of files) {
      const relativePath = file.webkitRelativePath || file.name;
      if (!knowledgeFilePattern.test(relativePath)) {
        unsupported.push(relativePath);
        continue;
      }
      documents.push({ path: relativePath, content: await file.text() });
    }
    if (documents.length === 0) {
      throw new Error(unsupported.length ? "No supported knowledge files found. Use Markdown, text, JSON, YAML, CSV, ZIP, or .skill.json." : "No files selected.");
    }
    const suffix = unsupported.length ? ` (${unsupported.length} unsupported file${unsupported.length === 1 ? "" : "s"} ignored)` : "";
    await ingestPayload({ documents }, `${documents.length} knowledge file${documents.length === 1 ? "" : "s"} added${suffix}.`);
  }, [ingestPayload]);

  const handleFileInput = useCallback(async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    event.target.value = "";
    try {
      await ingestFiles(files);
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Could not read selected files." });
    }
  }, [ingestFiles]);

  const handleDrop = useCallback(async (event: DragEvent<HTMLElement>) => {
    event.preventDefault();
    setDragging(false);
    try {
      await ingestFiles(Array.from(event.dataTransfer.files ?? []));
    } catch (error) {
      setNotice({ kind: "error", text: error instanceof Error ? error.message : "Could not read dropped files." });
    }
  }, [ingestFiles]);

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
    ? { title: "No Community Skills yet", copy: "Promoted Community Skills will appear here. Publishing remains a reviewed lifecycle, separate from quick Personal Skill creation." }
    : view === "mine"
      ? { title: "No Personal Skills yet", copy: "Paste a link or text above, or drop Markdown, JSON, YAML, CSV, ZIP, a folder, or an existing .skill.json package." }
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

      <section
        className={smart.smartAdd}
        data-dragging={dragging || undefined}
        onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
        onDragLeave={(event) => { if (event.currentTarget === event.target) setDragging(false); }}
        onDragOver={(event) => { event.preventDefault(); setDragging(true); }}
        onDrop={(event) => void handleDrop(event)}
      >
        <div className={smart.inputRow}>
          <textarea
            aria-label="Add a Skill from a link or text"
            className={smart.smartInput}
            disabled={busy || !resource.storageConfigured}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") void submitDraft();
            }}
            placeholder="Paste a GitHub/docs link, paste instructions, or describe knowledge you want CodeLocal to reuse…"
            value={draft}
          />
          <button className={smart.addButton} disabled={busy || !resource.storageConfigured || !draft.trim()} onClick={() => void submitDraft()} type="button">
            {busy ? "Adding…" : "Add Skill"}
          </button>
        </div>
        <div className={smart.actions}>
          <div className={smart.shortcuts}>
            <button className={smart.shortcut} disabled={busy || !resource.storageConfigured} onClick={() => fileInput.current?.click()} type="button">Files / ZIP</button>
            <button className={smart.shortcut} disabled={busy || !resource.storageConfigured} onClick={() => folderInput.current?.click()} type="button">Folder</button>
          </div>
          <div className={smart.secondaryActions}>
            <button className={smart.secondaryAction} disabled={busy || !resource.storageConfigured} onClick={() => communityInput.current?.click()} type="button">Publish package</button>
            <span className={smart.hint}><strong>{resource.storageConfigured ? "Personal Skills ready" : "Skill storage unavailable"}</strong></span>
          </div>
        </div>
        <p className={smart.hint}>Drop anything here. CodeLocal keeps quick imports <strong>Personal + Knowledge + Auto</strong>; source code is read as bounded knowledge only and is never executed during ingestion.</p>
        <input accept=".md,.txt,.json,.yaml,.yml,.csv,.zip,.skill.json,application/json,application/zip" hidden multiple onChange={(event) => void handleFileInput(event)} ref={fileInput} type="file" />
        <input hidden multiple onChange={(event) => void handleFileInput(event)} ref={folderInput} type="file" />
        <input accept=".json,.skill.json,application/json" hidden onChange={(event) => void uploadPackage(event, "/api/v1/skills/publish", "Community Skill submitted for review.")} ref={communityInput} type="file" />
        <input accept=".json,.skill.json,application/json" hidden onChange={(event) => void uploadPackage(event, "/api/v1/admin/skills/import", "Admin Skill candidate imported.")} ref={adminInput} type="file" />
      </section>

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
          <p>{view === "for-you" ? "No install ritual. Eligible Skills are selected directly from chat." : view === "mine" ? "Personal Skills follow your account across projects; project-specific facts still belong to Project Brain." : "Manage reusable expertise without weakening the existing Skill lifecycle and security gates."}</p>
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
