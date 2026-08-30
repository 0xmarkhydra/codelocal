"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isBlogPostResource } from "@/lib/contracts/blog";
import { useDashboardResource } from "../use-dashboard-resource";
import dashboard from "../dashboard.module.css";
import styles from "./blog-editor.module.css";

function blockText(content: unknown[]) {
  return content.map((block) => {
    if (typeof block !== "object" || block === null || Array.isArray(block)) return "";
    const item = block as Record<string, unknown>;
    if (typeof item.text === "string") return item.text;
    if (Array.isArray(item.items)) return item.items.filter((value): value is string => typeof value === "string").join("\n");
    if (typeof item.code === "string") return item.code;
    return "";
  }).filter(Boolean).join("\n\n");
}

function contentFromText(value: string) {
  return value.trim()
    ? value.split(/\n\s*\n/).map((text) => ({ type: "paragraph", text: text.trim() })).filter((block) => block.text)
    : [];
}

type SaveState = "idle" | "saving" | "saved" | "error";

export function BlogEditor({ postID }: { postID: string }) {
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const resource = useDashboardResource(`/api/v1/blog/posts/${encodeURIComponent(postID)}`, isBlogPostResource);
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [excerpt, setExcerpt] = useState("");
  const [category, setCategory] = useState("");
  const [tags, setTags] = useState("");
  const [visibility, setVisibility] = useState("public");
  const [body, setBody] = useState("");
  const [hydratedID, setHydratedID] = useState("");
  const [dirty, setDirty] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [actionError, setActionError] = useState("");

  const post = resource.state.kind === "ready" ? resource.state.value.post : undefined;

  useEffect(() => {
    if (!post || hydratedID === post.id) return;
    setTitle(post.title);
    setSlug(post.slug);
    setExcerpt(post.excerpt);
    setCategory(post.category ?? "");
    setTags(post.tags.join(", "));
    setVisibility(post.visibility);
    setBody(blockText(post.content));
    setHydratedID(post.id);
    setDirty(false);
    setSaveState("idle");
  }, [hydratedID, post]);

  const payload = useMemo(() => ({
    title,
    slug,
    excerpt,
    category,
    tags: tags.split(",").map((tag) => tag.trim()).filter(Boolean),
    visibility,
    content: contentFromText(body),
  }), [body, category, excerpt, slug, tags, title, visibility]);

  const save = useCallback(async () => {
    if (!post || !dirty || account.state.kind !== "ready") return true;
    setSaveState("saving");
    setActionError("");
    try {
      const response = await fetch(`/api/v1/blog/posts/${encodeURIComponent(post.id)}`, {
        method: "PATCH",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": account.state.value.csrf,
        },
        body: JSON.stringify(payload),
      });
      const responseBody: unknown = await response.json().catch(() => null);
      if (!response.ok || !isBlogPostResource(responseBody)) {
        setSaveState("error");
        setActionError(response.status === 409 ? "That URL slug is already in use." : `Save failed (${response.status}).`);
        return false;
      }
      setDirty(false);
      setSaveState("saved");
      return true;
    } catch (error) {
      setSaveState("error");
      setActionError(error instanceof Error ? error.message : "Save failed.");
      return false;
    }
  }, [account.state, dirty, payload, post]);

  useEffect(() => {
    if (!dirty || hydratedID !== post?.id) return;
    const timer = window.setTimeout(() => { void save(); }, 1200);
    return () => window.clearTimeout(timer);
  }, [dirty, hydratedID, post?.id, save]);

  function change(setter: (value: string) => void, value: string) {
    setter(value);
    setDirty(true);
    setSaveState("idle");
  }

  async function setPublished(published: boolean) {
    if (!post || account.state.kind !== "ready") return;
    if (!(await save())) return;
    setActionError("");
    try {
      const response = await fetch(`/api/v1/blog/posts/${encodeURIComponent(post.id)}/${published ? "publish" : "unpublish"}`, {
        method: "POST",
        credentials: "same-origin",
        headers: { Accept: "application/json", "X-CSRF-Token": account.state.value.csrf },
      });
      if (!response.ok) {
        setActionError(`Unable to ${published ? "publish" : "unpublish"} (${response.status}).`);
        return;
      }
      resource.retry();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Publishing action failed.");
    }
  }

  if (resource.state.kind === "loading") {
    return <section className={dashboard.content}><div className={styles.state}>Loading editor…</div></section>;
  }
  if (resource.state.kind !== "ready" || !post) {
    const message = resource.state.kind === "error" ? resource.state.message : "This post is not available.";
    return <section className={dashboard.content}><div className={styles.state}>{message}</div></section>;
  }

  return (
    <section className={dashboard.content}>
      <div className={styles.editor}>
        <header className={styles.topbar}>
          <Link href="/dashboard/blogs">← Blogs</Link>
          <span className={styles.saveState} aria-live="polite">
            {saveState === "saving" ? "Saving…" : saveState === "saved" ? "Saved" : saveState === "error" ? "Save failed" : dirty ? "Unsaved" : ""}
          </span>
          <div className={styles.actions}>
            {post.status === "published" && <Link href={`/blog/${post.slug}`}>View</Link>}
            <button type="button" onClick={() => void save()} disabled={!dirty || saveState === "saving"}>Save</button>
            <button className={styles.publish} type="button" onClick={() => void setPublished(post.status !== "published")}>
              {post.status === "published" ? "Unpublish" : "Publish"}
            </button>
          </div>
        </header>

        <main className={styles.canvas}>
          <input className={styles.title} maxLength={200} aria-label="Title" value={title} onChange={(event) => change(setTitle, event.target.value)} placeholder="Post title" />
          <textarea className={styles.excerpt} maxLength={700} aria-label="Excerpt" value={excerpt} onChange={(event) => change(setExcerpt, event.target.value)} placeholder="Short description for previews and SEO" rows={3} />
          <textarea className={styles.body} aria-label="Post body" value={body} onChange={(event) => change(setBody, event.target.value)} placeholder="Start writing…" rows={18} />
        </main>

        <aside className={styles.settings}>
          <h2>Post settings</h2>
          <label>URL slug<input value={slug} onChange={(event) => change(setSlug, event.target.value)} /></label>
          <label>Category<input maxLength={80} value={category} onChange={(event) => change(setCategory, event.target.value)} /></label>
          <label>Tags<input value={tags} onChange={(event) => change(setTags, event.target.value)} placeholder="AI, MCP, Agents" /></label>
          <label>Visibility
            <select value={visibility} onChange={(event) => change(setVisibility, event.target.value)}>
              <option value="public">Public</option>
              <option value="unlisted">Unlisted</option>
              <option value="private">Private</option>
            </select>
          </label>
          <div className={styles.mediaNotice}>
            <strong>Images</strong>
            <span>Durable S3 assets + WebP optimization are being wired next. Temporary 24h media is intentionally not used for Blog posts.</span>
          </div>
          {actionError && <p className={styles.error} role="alert">{actionError}</p>}
        </aside>
      </div>
    </section>
  );
}
