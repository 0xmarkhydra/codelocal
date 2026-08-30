"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useMemo, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isBlogPostResource, isBlogPostsResource } from "@/lib/contracts/blog";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./blogs.module.css";

function formatDate(value: number) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("en", { month: "short", day: "numeric", year: "numeric" }).format(new Date(value));
}

export function BlogsHub() {
  const router = useRouter();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const resource = useDashboardResource("/api/v1/blog/posts", isBlogPostsResource);
  const [title, setTitle] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const posts = resource.state.kind === "ready" ? resource.state.value.posts : [];
  const counts = useMemo(() => ({
    published: posts.filter((post) => post.status === "published").length,
    drafts: posts.filter((post) => post.status === "draft").length,
  }), [posts]);

  async function createDraft(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (creating || account.state.kind !== "ready" || !title.trim()) return;
    setCreating(true);
    setError("");
    try {
      const response = await fetch("/api/v1/blog/posts", {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": account.state.value.csrf,
        },
        body: JSON.stringify({ title: title.trim(), content: [] }),
      });
      const body: unknown = await response.json().catch(() => null);
      if (!response.ok || !isBlogPostResource(body)) {
        setError(response.status === 409 ? "A post already uses that slug. Try a more specific title." : `Unable to create draft (${response.status}).`);
        return;
      }
      router.push(`/dashboard/blogs/${body.post.id}`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to create draft.");
    } finally {
      setCreating(false);
    }
  }

  return (
    <>
      <div className={styles.stats} aria-label="Blog summary">
        <article><strong>{counts.published}</strong><span>Published</span></article>
        <article><strong>{counts.drafts}</strong><span>Drafts</span></article>
      </div>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <div><span className={styles.kicker}>Write</span><h2>New draft</h2></div>
        </div>
        <form className={styles.createForm} onSubmit={createDraft}>
          <input
            aria-label="Post title"
            maxLength={200}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="What do you want to write about?"
            value={title}
          />
          <button disabled={creating || account.state.kind !== "ready" || !title.trim()} type="submit">
            {creating ? "Creating…" : "Create draft"}
          </button>
        </form>
        {error && <p className={styles.errorText} role="alert">{error}</p>}
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <div><span className={styles.kicker}>Your content</span><h2>Posts</h2></div>
          <Link href="/blog">Open public blog</Link>
        </div>

        {resource.state.kind === "loading" && <div className={styles.emptyState}>Loading posts…</div>}
        {resource.state.kind === "error" && (
          <div className={styles.emptyState}>
            <p>{resource.state.message}</p>
            <button type="button" onClick={resource.retry}>Try again</button>
          </div>
        )}
        {resource.state.kind === "unauthenticated" && <div className={styles.emptyState}>Sign in again to manage your posts.</div>}
        {resource.state.kind === "ready" && posts.length === 0 && <div className={styles.emptyState}>No posts yet. Create your first draft above.</div>}
        {resource.state.kind === "ready" && posts.length > 0 && (
          <div className={styles.postList}>
            {posts.map((post) => (
              <Link className={styles.postRow} href={`/dashboard/blogs/${post.id}`} key={post.id}>
                <div>
                  <span>{post.status} · {post.category || "Uncategorized"}</span>
                  <strong>{post.title}</strong>
                  <p>{post.excerpt || "No excerpt yet."}</p>
                </div>
                <time dateTime={new Date(post.updatedAt).toISOString()}>{formatDate(post.updatedAt)}</time>
              </Link>
            ))}
          </div>
        )}
      </section>
    </>
  );
}
