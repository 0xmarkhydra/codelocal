"use client";

import Image from "next/image";
import Link from "next/link";
import { ChangeEvent, FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { privateMediaVariantURL, uploadMediaAsset } from "@/lib/media-upload";
import { AppIcon } from "../app-icon";
import { useDashboardResource } from "../use-dashboard-resource";
import { ForumKind, ForumStatus, ForumTopic, formatForumTime, isForumTopic, kindLabels, statusLabels } from "./forum-types";
import styles from "./forums.module.css";

type TopicListResponse = { topics: ForumTopic[]; isAdmin: boolean };

type Draft = {
  kind: ForumKind;
  title: string;
  body: string;
  severity: "" | "low" | "medium" | "high" | "critical";
  version: string;
  environment: string;
  reproductionSteps: string;
  expectedBehavior: string;
  actualBehavior: string;
  tags: string;
};

const emptyDraft: Draft = {
  kind: "question",
  title: "",
  body: "",
  severity: "medium",
  version: "",
  environment: "",
  reproductionSteps: "",
  expectedBehavior: "",
  actualBehavior: "",
  tags: "",
};

function validList(value: unknown): value is TopicListResponse {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return Array.isArray(item.topics) && item.topics.every(isForumTopic) && typeof item.isAdmin === "boolean";
}

export function ForumsHub() {
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const [topics, setTopics] = useState<ForumTopic[]>([]);
  const [isAdmin, setIsAdmin] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<"all" | ForumKind>("all");
  const [status, setStatus] = useState<"all" | ForumStatus>("all");
  const [composerOpen, setComposerOpen] = useState(false);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [assetIDs, setAssetIDs] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    const params = new URLSearchParams();
    if (kind !== "all") params.set("kind", kind);
    if (status !== "all") params.set("status", status);
    if (query.trim()) params.set("q", query.trim());
    try {
      const response = await fetch(`/api/v1/forums/topics?${params.toString()}`, { cache: "no-store", credentials: "same-origin" });
      if (!response.ok) throw new Error("Unable to load forums");
      const payload: unknown = await response.json();
      if (!validList(payload)) throw new Error("Invalid forum response");
      setTopics(payload.topics);
      setIsAdmin(payload.isAdmin);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Unable to load forums");
    } finally {
      setLoading(false);
    }
  }, [kind, query, status]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 180);
    return () => window.clearTimeout(timer);
  }, [load]);

  const counts = useMemo(() => ({
    question: topics.filter((topic) => topic.kind === "question").length,
    bug: topics.filter((topic) => topic.kind === "bug").length,
    idea: topics.filter((topic) => topic.kind === "idea").length,
  }), [topics]);

  async function addImages(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.currentTarget.files ?? []);
    event.currentTarget.value = "";
    if (!readyAccount?.csrf || files.length === 0 || uploading) return;
    const remaining = Math.max(0, 6 - assetIDs.length);
    if (remaining === 0) return;
    setUploading(true);
    setError("");
    try {
      const next = [...assetIDs];
      for (const file of files.slice(0, remaining)) {
        const asset = await uploadMediaAsset(file, readyAccount.csrf);
        if (!next.includes(asset.id)) next.push(asset.id);
      }
      setAssetIDs(next);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not upload image");
    } finally {
      setUploading(false);
    }
  }

  async function createTopic(event: FormEvent) {
    event.preventDefault();
    if (!readyAccount?.csrf || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      const response = await fetch("/api/v1/forums/topics", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": readyAccount.csrf },
        body: JSON.stringify({
          ...draft,
          tags: draft.tags.split(",").map((tag) => tag.trim()).filter(Boolean),
          assetIds: assetIDs,
        }),
      });
      if (!response.ok) throw new Error("Could not create topic");
      const payload = await response.json() as { topic?: ForumTopic };
      if (!payload.topic || !isForumTopic(payload.topic)) throw new Error("Invalid created topic");
      setDraft(emptyDraft);
      setAssetIDs([]);
      setComposerOpen(false);
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not create topic");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>CodeLocal Community</span>
          <h1>Forums</h1>
          <p>Ask for help, report bugs, and propose ideas. Verified bugs can move directly into the admin review and GitHub issue workflow.</p>
        </div>
        <button className={styles.primaryButton} type="button" onClick={() => setComposerOpen((open) => !open)}>
          <AppIcon name={composerOpen ? "close" : "plus"} size={17} />
          {composerOpen ? "Close" : "New topic"}
        </button>
      </header>

      <section className={styles.kindGrid} aria-label="Forum categories">
        {(["question", "bug", "idea"] as ForumKind[]).map((item) => (
          <button key={item} className={styles.kindCard} data-active={kind === item || undefined} type="button" onClick={() => setKind((current) => current === item ? "all" : item)}>
            <span className={styles.kindIcon}><AppIcon name={item === "bug" ? "shield" : item === "idea" ? "brain" : "forum"} size={19} /></span>
            <span><strong>{kindLabels[item]}</strong><small>{item === "question" ? "Get help from the community" : item === "bug" ? "Reproduce, review, fix" : "Shape the roadmap"}</small></span>
            <b>{counts[item]}</b>
          </button>
        ))}
      </section>

      {composerOpen && (
        <form className={styles.composer} onSubmit={createTopic}>
          <div className={styles.composerHead}><div><span className={styles.eyebrow}>Create discussion</span><h2>Describe the problem clearly</h2></div>{isAdmin && <span className={styles.adminBadge}>Admin</span>}</div>
          <div className={styles.kindPicker}>
            {(["question", "bug", "idea"] as ForumKind[]).map((item) => <button type="button" key={item} data-active={draft.kind === item || undefined} onClick={() => setDraft((current) => ({ ...current, kind: item }))}>{kindLabels[item]}</button>)}
          </div>
          <label><span>Title</span><input required maxLength={180} value={draft.title} onChange={(event) => setDraft((current) => ({ ...current, title: event.target.value }))} placeholder="Short, specific title" /></label>
          <label><span>Description</span><textarea required rows={6} value={draft.body} onChange={(event) => setDraft((current) => ({ ...current, body: event.target.value }))} placeholder="What happened? What are you trying to do?" /></label>
          <div className={styles.attachments}>
            <div className={styles.attachmentHead}>
              <div><strong>Screenshots / images</strong><span>JPEG, PNG or WebP · up to 6 images · 25 MB each</span></div>
              <label className={styles.fileButton}>
                {uploading ? "Uploading…" : "Add images"}
                <input type="file" accept="image/jpeg,image/png,image/webp" multiple disabled={uploading || !readyAccount || assetIDs.length >= 6} onChange={(event) => void addImages(event)} />
              </label>
            </div>
            {assetIDs.length > 0 && <div className={styles.attachmentGrid}>{assetIDs.map((assetID) => <div className={styles.attachment} key={assetID}><Image src={privateMediaVariantURL(assetID, "thumb")} alt="Forum attachment preview" width={180} height={120} unoptimized /><button type="button" aria-label="Remove image" onClick={() => setAssetIDs((current) => current.filter((id) => id !== assetID))}>×</button></div>)}</div>}
          </div>
          {draft.kind === "bug" && <div className={styles.bugFields}>
            <label><span>Severity</span><select value={draft.severity} onChange={(event) => setDraft((current) => ({ ...current, severity: event.target.value as Draft["severity"] }))}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label>
            <label><span>CodeLocal version</span><input value={draft.version} onChange={(event) => setDraft((current) => ({ ...current, version: event.target.value }))} placeholder="e.g. 1.5.65" /></label>
            <label className={styles.wide}><span>Environment</span><textarea rows={3} value={draft.environment} onChange={(event) => setDraft((current) => ({ ...current, environment: event.target.value }))} placeholder="OS, device, browser/client, workspace state…" /></label>
            <label className={styles.wide}><span>Steps to reproduce</span><textarea rows={4} value={draft.reproductionSteps} onChange={(event) => setDraft((current) => ({ ...current, reproductionSteps: event.target.value }))} placeholder="1. Open… 2. Click… 3. See error…" /></label>
            <label><span>Expected</span><textarea rows={3} value={draft.expectedBehavior} onChange={(event) => setDraft((current) => ({ ...current, expectedBehavior: event.target.value }))} /></label>
            <label><span>Actual</span><textarea rows={3} value={draft.actualBehavior} onChange={(event) => setDraft((current) => ({ ...current, actualBehavior: event.target.value }))} /></label>
          </div>}
          <label><span>Tags</span><input value={draft.tags} onChange={(event) => setDraft((current) => ({ ...current, tags: event.target.value }))} placeholder="mcp, macos, plugins (comma separated)" /></label>
          <div className={styles.composerActions}><button type="button" className={styles.secondaryButton} onClick={() => setComposerOpen(false)}>Cancel</button><button className={styles.primaryButton} disabled={!readyAccount || submitting || uploading} type="submit">{submitting ? "Publishing…" : uploading ? "Uploading…" : "Publish topic"}</button></div>
        </form>
      )}

      <section className={styles.toolbar}>
        <label className={styles.search}><AppIcon name="search" size={17} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search discussions" /></label>
        <select value={kind} onChange={(event) => setKind(event.target.value as "all" | ForumKind)}><option value="all">All types</option><option value="question">Questions</option><option value="bug">Bugs</option><option value="idea">Ideas</option></select>
        <select value={status} onChange={(event) => setStatus(event.target.value as "all" | ForumStatus)}><option value="all">All statuses</option>{Object.entries(statusLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>
      </section>

      {error && <div className={styles.error}><AppIcon name="shield" size={17} />{error}<button type="button" onClick={() => void load()}>Retry</button></div>}
      {loading ? <div className={styles.empty}>Loading discussions…</div> : topics.length === 0 ? <div className={styles.empty}><AppIcon name="forum" size={30} /><strong>No discussions yet</strong><span>Start the first topic for this filter.</span></div> : (
        <div className={styles.topicList}>
          {topics.map((topic) => (
            <Link className={styles.topicCard} href={`/dashboard/forums/${encodeURIComponent(topic.id)}`} key={topic.id}>
              <div className={styles.voteBox}><strong>{topic.voteCount}</strong><span>votes</span></div>
              <div className={styles.topicBody}>
                <div className={styles.topicMeta}><span className={styles.kindBadge} data-kind={topic.kind}>{kindLabels[topic.kind]}</span><span className={styles.statusBadge} data-status={topic.status}>{statusLabels[topic.status]}</span>{topic.kind === "bug" && topic.severity && <span className={styles.severity} data-severity={topic.severity}>{topic.severity}</span>}</div>
                <h2>{topic.title}</h2>
                <p>{topic.body}</p>
                <div className={styles.tags}>{topic.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>
                <footer><span>{topic.authorEmail}</span><span>Updated {formatForumTime(topic.updatedAt)}</span><span>{topic.commentCount} replies</span>{topic.githubIssueNumber ? <span>GitHub #{topic.githubIssueNumber}</span> : null}</footer>
              </div>
              <AppIcon className={styles.chevron} name="chevron-right" size={17} />
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
