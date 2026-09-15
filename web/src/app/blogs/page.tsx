import Link from "next/link";
import { EmptyState, PostCard, SeriesCard } from "../blog/_components";
import styles from "../blog/blog.module.css";
import { taxonomySlug } from "@/lib/blog";
import { getTranslations } from "@/lib/i18n/server";
import {
  getBlogCategoriesForRender,
  getBlogPostsForRender,
  getBlogSeriesForRender,
  getBlogTagsForRender,
} from "@/lib/blog-server";

type BlogHomeProps = {
  searchParams: Promise<{ q?: string | string[] }>;
};

export default async function BlogHome({ searchParams }: BlogHomeProps) {
  const t = await getTranslations();
  const params = await searchParams;
  const queryValue = Array.isArray(params.q) ? params.q[0] : params.q;
  const query = queryValue?.trim() ?? "";
  const [allPosts, series, categories, tags] = await Promise.all([
    getBlogPostsForRender(),
    getBlogSeriesForRender(),
    getBlogCategoriesForRender(),
    getBlogTagsForRender(),
  ]);
  const normalized = query.toLowerCase();
  const posts = normalized
    ? allPosts.filter((post) => [post.title, post.excerpt, post.category, ...post.tags].join(" ").toLowerCase().includes(normalized))
    : allPosts;
  const featured = query ? [] : allPosts.filter((post) => post.featured).slice(0, 4);

  return (
    <main className={styles.page}>
      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <h1>{t("CodeLocal.Cloud Blog")}</h1>
          <p>{t("Practical notes on local-first AI infrastructure, agents and project intelligence.")}</p>
        </div>
        <aside className={styles.heroAside}>
          <form className={styles.searchForm} action="/blogs" method="get">
            <label className="sr-only" htmlFor="blog-search">{t("Search the CodeLocal.Cloud Blog")}</label>
            <input
              id="blog-search"
              name="q"
              type="search"
              defaultValue={query}
              placeholder={t("Search articles")}
            />
            <button type="submit">{t("Search")}</button>
          </form>
        </aside>
      </section>

      {featured.length > 0 && (
        <section className={styles.section} aria-labelledby="featured-posts">
          <div className={styles.sectionHead}>
            <div>
              <h2 id="featured-posts">{t("Featured articles")}</h2>
            </div>
          </div>
          <div className={styles.postGrid}>
            {featured.map((post) => <PostCard key={post.slug} post={post} />)}
          </div>
        </section>
      )}

      <section className={styles.section} aria-labelledby="latest-posts">
        <div className={styles.sectionHead}>
          <div>
            <h2 id="latest-posts">{query ? t("Results for “{query}”", { query }) : t("All articles")}</h2>
          </div>
          <p>{t("{count} articles", { count: posts.length })}</p>
        </div>
        {posts.length > 0 ? (
          <div className={styles.postGrid}>
            {posts.map((post) => <PostCard key={post.slug} post={post} compact />)}
          </div>
        ) : <EmptyState query={query} />}
      </section>

      {!query && series.length > 0 && (
        <section className={styles.section} aria-labelledby="blog-series">
          <div className={styles.sectionHead}>
            <div>
              <h2 id="blog-series">{t("Series")}</h2>
            </div>
            <Link className={styles.readMore} href="/blogs/series">{t("View all series")} <span aria-hidden="true">→</span></Link>
          </div>
          <div className={styles.seriesGrid}>
            {series.slice(0, 6).map((item) => (
              <SeriesCard key={item.slug} series={item} posts={allPosts.filter((post) => post.series?.slug === item.slug)} />
            ))}
          </div>
        </section>
      )}

      {!query && (
        <section className={styles.section} aria-labelledby="explore-blog">
          <div className={styles.sectionHead}>
            <div>
              <h2 id="explore-blog">{t("Topics")}</h2>
            </div>
            <p>{t("Browse by category or tag")}</p>
          </div>
          <div className={styles.taxonomyCloud}>
            {categories.map((category) => (
              <Link key={category} href={`/blogs/category/${taxonomySlug(category)}`}>{category}</Link>
            ))}
            {tags.map((tag) => (
              <Link key={tag} href={`/blogs/tag/${taxonomySlug(tag)}`}>#{tag}</Link>
            ))}
          </div>
        </section>
      )}
    </main>
  );
}
