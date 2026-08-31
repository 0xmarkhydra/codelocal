"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useMemo, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import {
  isBlogPostResource,
  isBlogPostsResource,
  isBlogSeriesCollectionResource,
  isBlogSeriesResource,
} from "@/lib/contracts/blog";
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
  const seriesResource = useDashboardResource("/api/v1/blog/series", isBlogSeriesCollectionResource);
  const [title, setTitle] = useState("");
  const [seriesTitle, setSeriesTitle] = useState("");
  const [creating, setCreating] = useState(false);
  const [creatingSeries, setCreatingSeries] = useState(false);
  const [error, setError] = useState("");
  const [seriesError, setSeriesError] = useState("");
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "published" | "draft">("all");

  const posts = useMemo(() => resource.state.kind === "ready" ? resource.state.value.posts : [], [resource.state]);
  const series = useMemo(() => seriesResource.state.kind === "ready" ? seriesResource.state.value.series : [], [seriesResource.state]);
  const counts = useMemo(() => ({
    published: posts.filter((post) => post.status === "published").length,
    drafts: posts.filter((post) => post.status === "draft").length,
    series: series.filter((item) => item.status !== "archived").length,
  }), [posts, series]);
  const filteredPosts = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return posts.filter((post) => {
      if (statusFilter !== "all" && post.status !== statusFilter) return false;
      if (!normalized) return true;
      return [post.title, post.excerpt, post.category, ...post.tags]
        .filter((value): value is string => typeof value === "string")
        .some((value) => value.toLowerCase().includes(normalized));
    });
  }, [posts, query, statusFilter]);

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

  async function createSeries(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (creatingSeries || account.state.kind !== "ready" || !seriesTitle.trim()) return;
    setCreatingSeries(true);
    setSeriesError("");
    try {
      const response = await fetch("/api/v1/blog/series", {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": account.state.value.csrf,
        },
        body: JSON.stringify({ title: seriesTitle.trim() }),
      });
      const body: unknown = await response.json().catch(() => null);
      if (!response.ok || !isBlogSeriesResource(body)) {
        setSeriesError(response.status === 409 ? "A series already uses that slug. Try a more specific title." : `Unable to create series (${response.status}).`);
        return;
      }
      router.push(`/dashboard/blogs/series/${body.series.id}`);
    } catch (caught) {
      setSeriesError(caught instanceof Error ? caught.message : "Unable to create series.");
    } finally {
      setCreatingSeries(false);
    }
  }

  return (
    <div className={styles.blogWorkspace}>
      <section className={`${styles.section} ${styles.postsSection}`}>
        <div className={styles.sectionHeader}>
          <div><span className={styles.kicker}>Your content</span><h2>Posts</h2></div>
          <Link href="/blogs">Open public blog</Link>
        </div>

        <div className={styles.postToolbar}>
          <input
            aria-label="Search posts"
            className={styles.searchInput}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search title, excerpt, category or tag"
            type="search"
            value={query}
          />
          <div className={styles.statusFilters} role="group" aria-label="Filter posts by status">
            {(["all", "published", "draft"] as const).map((status) => (
              <button
                aria-pressed={statusFilter === status}
                data-active={statusFilter === status}
                key={status}
                onClick={() => setStatusFilter(status)}
                type="button"
              >
                {status === "all" ? "All" : status === "published" ? "Published" : "Drafts"}
              </button>
            ))}
          </div>
        </div>

        {resource.state.kind === "loading" && <div className={styles.emptyState}>Loading posts…</div>}
        {resource.state.kind === "error" && (
          <div className={styles.emptyState}>
            <p>{resource.state.message}</p>
            <button type="button" onClick={resource.retry}>Try again</button>
          </div>
        )}
        {resource.state.kind === "unauthenticated" && <div className={styles.emptyState}>Sign in again to manage your posts.</div>}
        {resource.state.kind === "ready" && posts.length === 0 && <div className={styles.emptyState}>No posts yet. Create your first draft from the right panel.</div>}
        {resource.state.kind === "ready" && posts.length > 0 && filteredPosts.length === 0 && <div className={styles.emptyState}>No posts match this search or filter.</div>}
        {resource.state.kind === "ready" && filteredPosts.length > 0 && (
          <div className={styles.postList}>
            {filteredPosts.map((post) => (
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

      <aside className={styles.sideRail}>
        <div className={styles.stats} aria-label="Blog summary">
          <article><strong>{counts.published}</strong><span>Published</span></article>
          <article><strong>{counts.drafts}</strong><span>Drafts</span></article>
          <article><strong>{counts.series}</strong><span>Series</span></article>
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
            <div><span className={styles.kicker}>Learning paths</span><h2>Series</h2></div>
          </div>
          <form className={styles.createForm} onSubmit={createSeries}>
            <input
              aria-label="Series title"
              maxLength={200}
              onChange={(event) => setSeriesTitle(event.target.value)}
              placeholder="Create a new series"
              value={seriesTitle}
            />
            <button disabled={creatingSeries || account.state.kind !== "ready" || !seriesTitle.trim()} type="submit">
              {creatingSeries ? "Creating…" : "Create series"}
            </button>
          </form>
          {seriesError && <p className={styles.errorText} role="alert">{seriesError}</p>}
          {seriesResource.state.kind === "loading" && <div className={styles.emptyState}>Loading series…</div>}
          {seriesResource.state.kind === "error" && <div className={styles.emptyState}><p>{seriesResource.state.message}</p><button type="button" onClick={seriesResource.retry}>Try again</button></div>}
          {seriesResource.state.kind === "ready" && series.length === 0 && <div className={styles.emptyState}>No series yet.</div>}
          {seriesResource.state.kind === "ready" && series.length > 0 && (
            <div className={styles.postList}>
              {series.map((item) => (
                <Link className={styles.postRow} href={`/dashboard/blogs/series/${item.id}`} key={item.id}>
                  <div>
                    <span>{item.status}</span>
                    <strong>{item.title}</strong>
                    <p>{item.description || "No description yet."}</p>
                  </div>
                  <time dateTime={new Date(item.updatedAt).toISOString()}>{formatDate(item.updatedAt)}</time>
                </Link>
              ))}
            </div>
          )}
        </section>
      </aside>
    </div>
  );
}
