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
import { AppIcon } from "../app-icon";
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

  const posts = resource.state.kind === "ready" ? resource.state.value.posts : [];
  const series = seriesResource.state.kind === "ready" ? seriesResource.state.value.series : [];
  const counts = useMemo(() => ({
    all: posts.length,
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
      <section className={styles.overview} aria-label="Blog overview">
        <div className={styles.stats}>
          <article><strong>{counts.all}</strong><span>All posts</span></article>
          <article><strong>{counts.published}</strong><span>Published</span></article>
          <article><strong>{counts.drafts}</strong><span>Drafts</span></article>
          <article><strong>{counts.series}</strong><span>Series</span></article>
        </div>

        <div className={styles.quickGrid}>
          <form className={styles.quickCard} onSubmit={createDraft}>
            <div className={styles.quickCopy}>
              <span className={styles.quickIcon} aria-hidden="true"><AppIcon name="edit" size={17} /></span>
              <div>
                <span className={styles.kicker}>New article</span>
                <strong>Start a draft</strong>
                <p>Create the post first, then add metadata, media and publishing settings in the editor.</p>
              </div>
            </div>
            <div className={styles.quickForm}>
              <input
                aria-label="Post title"
                maxLength={200}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="Article title"
                value={title}
              />
              <button disabled={creating || account.state.kind !== "ready" || !title.trim()} type="submit">
                <AppIcon name="plus" size={14} />
                {creating ? "Creating…" : "Create draft"}
              </button>
            </div>
            {error && <p className={styles.errorText} role="alert">{error}</p>}
          </form>

          <form className={styles.quickCard} onSubmit={createSeries}>
            <div className={styles.quickCopy}>
              <span className={styles.quickIcon} aria-hidden="true"><AppIcon name="module" size={17} /></span>
              <div>
                <span className={styles.kicker}>Collection</span>
                <strong>Create a series</strong>
                <p>Group related posts into a learning path without changing their canonical article URLs.</p>
              </div>
            </div>
            <div className={styles.quickForm}>
              <input
                aria-label="Series title"
                maxLength={200}
                onChange={(event) => setSeriesTitle(event.target.value)}
                placeholder="Series title"
                value={seriesTitle}
              />
              <button disabled={creatingSeries || account.state.kind !== "ready" || !seriesTitle.trim()} type="submit">
                <AppIcon name="plus" size={14} />
                {creatingSeries ? "Creating…" : "Create series"}
              </button>
            </div>
            {seriesError && <p className={styles.errorText} role="alert">{seriesError}</p>}
          </form>
        </div>
      </section>

      <section className={`${styles.section} ${styles.postsSection}`}>
        <div className={styles.sectionHeader}>
          <div>
            <span className={styles.kicker}>Content library</span>
            <h2>Posts</h2>
            <p>Search, review and continue editing all of your articles.</p>
          </div>
          <span className={styles.sectionCount}>{filteredPosts.length} shown</span>
        </div>

        <div className={styles.postToolbar}>
          <label className={styles.searchBox}>
            <AppIcon name="search" size={15} />
            <input
              aria-label="Search posts"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search title, excerpt, category or tag"
              type="search"
              value={query}
            />
          </label>
          <div className={styles.statusFilters} role="group" aria-label="Filter posts by status">
            {(["all", "published", "draft"] as const).map((status) => (
              <button
                aria-pressed={statusFilter === status}
                data-active={statusFilter === status}
                key={status}
                onClick={() => setStatusFilter(status)}
                type="button"
              >
                {status === "all" ? `All ${counts.all}` : status === "published" ? `Published ${counts.published}` : `Drafts ${counts.drafts}`}
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
        {resource.state.kind === "ready" && posts.length === 0 && <div className={styles.emptyState}>No posts yet. Start a draft above.</div>}
        {resource.state.kind === "ready" && posts.length > 0 && filteredPosts.length === 0 && <div className={styles.emptyState}>No posts match this search or filter.</div>}
        {resource.state.kind === "ready" && filteredPosts.length > 0 && (
          <div className={styles.postList}>
            {filteredPosts.map((post) => (
              <Link className={styles.postRow} href={`/dashboard/blogs/${post.id}`} key={post.id}>
                <div className={styles.postMain}>
                  <div className={styles.postMeta}>
                    <span className={styles.statusChip} data-status={post.status}>{post.status}</span>
                    <span>{post.category || "Uncategorized"}</span>
                  </div>
                  <strong>{post.title}</strong>
                  <p>{post.excerpt || "No excerpt yet."}</p>
                </div>
                <div className={styles.postEnd}>
                  <time dateTime={new Date(post.updatedAt).toISOString()}>{formatDate(post.updatedAt)}</time>
                  <AppIcon name="chevron-right" size={15} />
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <div>
            <span className={styles.kicker}>Collections</span>
            <h2>Series</h2>
            <p>Organize related articles into a clear reading order.</p>
          </div>
          <span className={styles.sectionCount}>{counts.series} active</span>
        </div>

        {seriesResource.state.kind === "loading" && <div className={styles.emptyState}>Loading series…</div>}
        {seriesResource.state.kind === "error" && <div className={styles.emptyState}><p>{seriesResource.state.message}</p><button type="button" onClick={seriesResource.retry}>Try again</button></div>}
        {seriesResource.state.kind === "ready" && series.length === 0 && <div className={styles.emptyState}>No series yet. Create one above when you need a multi-part guide.</div>}
        {seriesResource.state.kind === "ready" && series.length > 0 && (
          <div className={styles.seriesGrid}>
            {series.map((item) => (
              <Link className={styles.seriesCard} href={`/dashboard/blogs/series/${item.id}`} key={item.id}>
                <div>
                  <span className={styles.statusChip} data-status={item.status}>{item.status}</span>
                  <strong>{item.title}</strong>
                  <p>{item.description || "No description yet."}</p>
                </div>
                <div className={styles.seriesFoot}>
                  <time dateTime={new Date(item.updatedAt).toISOString()}>Updated {formatDate(item.updatedAt)}</time>
                  <AppIcon name="chevron-right" size={15} />
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
