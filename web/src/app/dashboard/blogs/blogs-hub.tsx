"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useMemo, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey, MessageValues } from "@/lib/i18n/messages";
import {
  isBlogPostResource,
  isBlogPostsResource,
  isBlogSeriesCollectionResource,
  isBlogSeriesResource,
} from "@/lib/contracts/blog";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./blogs.module.css";

type Notice = { key: MessageKey; values?: MessageValues } | null;
const statusLabels = { draft: "Draft", scheduled: "Scheduled", published: "Published", archived: "Archived", active: "Active", complete: "Complete" } as const;

function formatDate(value: number, locale: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(locale, { month: "short", day: "numeric", year: "numeric" }).format(new Date(value));
}

export function BlogsHub() {
  const { locale, t, message } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  const router = useRouter();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const resource = useDashboardResource("/api/v1/blog/posts", isBlogPostsResource);
  const seriesResource = useDashboardResource("/api/v1/blog/series", isBlogSeriesCollectionResource);
  const [title, setTitle] = useState("");
  const [seriesTitle, setSeriesTitle] = useState("");
  const [creating, setCreating] = useState(false);
  const [creatingSeries, setCreatingSeries] = useState(false);
  const [error, setError] = useState<Notice>(null);
  const [seriesError, setSeriesError] = useState<Notice>(null);
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
    setError(null);
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
        setError(response.status === 409
          ? { key: "A post already uses that slug. Try a more specific title." }
          : { key: "Unable to create draft ({status}).", values: { status: String(response.status) } });
        return;
      }
      router.push(`/dashboard/blogs/${body.post.id}`);
    } catch {
      setError({ key: "Unable to create draft." });
    } finally {
      setCreating(false);
    }
  }

  async function createSeries(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (creatingSeries || account.state.kind !== "ready" || !seriesTitle.trim()) return;
    setCreatingSeries(true);
    setSeriesError(null);
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
        setSeriesError(response.status === 409
          ? { key: "A series already uses that slug. Try a more specific title." }
          : { key: "Unable to create series ({status}).", values: { status: String(response.status) } });
        return;
      }
      router.push(`/dashboard/blogs/series/${body.series.id}`);
    } catch {
      setSeriesError({ key: "Unable to create series." });
    } finally {
      setCreatingSeries(false);
    }
  }

  return (
    <div className={styles.blogWorkspace}>
      <section className={`${styles.section} ${styles.postsSection}`}>
        <div className={styles.sectionHeader}>
          <div><h2>{t("Posts")}</h2></div>
          <Link href="/blogs">{t("Open public blog")}</Link>
        </div>

        <div className={styles.postToolbar}>
          <input
            aria-label={t("Search posts")}
            className={styles.searchInput}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("Search title, excerpt, category or tag")}
            type="search"
            value={query}
          />
          <div className={styles.statusFilters} role="group" aria-label={t("Filter posts by status")}>
            {(["all", "published", "draft"] as const).map((status) => (
              <button
                aria-pressed={statusFilter === status}
                data-active={statusFilter === status}
                key={status}
                onClick={() => setStatusFilter(status)}
                type="button"
              >
                {t(status === "all" ? "All" : status === "published" ? "Published" : "Drafts")}
              </button>
            ))}
          </div>
        </div>

        {resource.state.kind === "loading" && <div className={styles.emptyState}>{t("Loading posts…")}</div>}
        {resource.state.kind === "error" && (
          <div className={styles.emptyState}>
            <p>{message(resource.state.message)}</p>
            <button type="button" onClick={resource.retry}>{t("Try again")}</button>
          </div>
        )}
        {resource.state.kind === "unauthenticated" && <div className={styles.emptyState}>{t("Sign in again to manage your posts.")}</div>}
        {resource.state.kind === "ready" && posts.length === 0 && <div className={styles.emptyState}>{t("No posts yet.")}</div>}
        {resource.state.kind === "ready" && posts.length > 0 && filteredPosts.length === 0 && <div className={styles.emptyState}>{t("No posts match this search or filter.")}</div>}
        {resource.state.kind === "ready" && filteredPosts.length > 0 && (
          <div className={styles.postList}>
            {filteredPosts.map((post) => (
              <Link className={styles.postRow} href={`/dashboard/blogs/${post.id}`} key={post.id}>
                <div>
                  <span>{t(statusLabels[post.status])} · {post.category || t("Uncategorized")}</span>
                  <strong>{post.title}</strong>
                  <p>{post.excerpt || t("No excerpt yet.")}</p>
                </div>
                <time dateTime={new Date(post.updatedAt).toISOString()}>{formatDate(post.updatedAt, locale)}</time>
              </Link>
            ))}
          </div>
        )}
      </section>

      <aside className={styles.sideRail}>
        <div className={styles.stats} aria-label={t("Blog summary")}>
          <article><strong>{resource.state.kind === "ready" ? number.format(counts.published) : "—"}</strong><span>{t("Published")}</span></article>
          <article><strong>{resource.state.kind === "ready" ? number.format(counts.drafts) : "—"}</strong><span>{t("Drafts")}</span></article>
          <article><strong>{seriesResource.state.kind === "ready" ? number.format(counts.series) : "—"}</strong><span>{t("Series")}</span></article>
        </div>

        <section className={styles.section}>
          <div className={styles.sectionHeader}>
            <div><h2>{t("New draft")}</h2></div>
          </div>
          <form className={styles.createForm} onSubmit={createDraft}>
            <input
              aria-label={t("Post title")}
              disabled={creating}
              maxLength={200}
              onChange={(event) => setTitle(event.target.value)}
              placeholder={t("Post title")}
              value={title}
            />
            <button disabled={creating || account.state.kind !== "ready" || !title.trim()} type="submit">
              {t(creating ? "Creating…" : "Create draft")}
            </button>
          </form>
          {error && <p className={styles.errorText} role="alert">{t(error.key, error.values)}</p>}
        </section>

        <section className={styles.section}>
          <div className={styles.sectionHeader}>
            <div><h2>{t("Series")}</h2></div>
          </div>
          <form className={styles.createForm} onSubmit={createSeries}>
            <input
              aria-label={t("Series title")}
              disabled={creatingSeries}
              maxLength={200}
              onChange={(event) => setSeriesTitle(event.target.value)}
              placeholder={t("Series title")}
              value={seriesTitle}
            />
            <button disabled={creatingSeries || account.state.kind !== "ready" || !seriesTitle.trim()} type="submit">
              {t(creatingSeries ? "Creating…" : "Create series")}
            </button>
          </form>
          {seriesError && <p className={styles.errorText} role="alert">{t(seriesError.key, seriesError.values)}</p>}
          {seriesResource.state.kind === "loading" && <div className={styles.emptyState}>{t("Loading series…")}</div>}
          {seriesResource.state.kind === "unauthenticated" && <div className={styles.emptyState}>{t("Sign in again to manage your series.")}</div>}
          {seriesResource.state.kind === "error" && <div className={styles.emptyState}><p>{message(seriesResource.state.message)}</p><button type="button" onClick={seriesResource.retry}>{t("Try again")}</button></div>}
          {seriesResource.state.kind === "ready" && series.length === 0 && <div className={styles.emptyState}>{t("No series yet.")}</div>}
          {seriesResource.state.kind === "ready" && series.length > 0 && (
            <div className={styles.postList}>
              {series.map((item) => (
                <Link className={styles.postRow} href={`/dashboard/blogs/series/${item.id}`} key={item.id}>
                  <div>
                    <span>{t(statusLabels[item.status])}</span>
                    <strong>{item.title}</strong>
                    <p>{item.description || t("No description yet.")}</p>
                  </div>
                  <time dateTime={new Date(item.updatedAt).toISOString()}>{formatDate(item.updatedAt, locale)}</time>
                </Link>
              ))}
            </div>
          )}
        </section>
      </aside>
    </div>
  );
}
