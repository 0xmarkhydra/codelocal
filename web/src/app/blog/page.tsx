import Link from "next/link";
import { EmptyState, PostCard, SeriesCard } from "./_components";
import styles from "./blog.module.css";
import {
  blogPosts,
  blogSeries,
  getCategories,
  getTags,
  searchBlogPosts,
  taxonomySlug,
} from "@/lib/blog";

type BlogHomeProps = {
  searchParams: Promise<{ q?: string | string[] }>;
};

export default async function BlogHome({ searchParams }: BlogHomeProps) {
  const params = await searchParams;
  const queryValue = Array.isArray(params.q) ? params.q[0] : params.q;
  const query = queryValue?.trim() ?? "";
  const posts = query ? searchBlogPosts(query) : blogPosts;
  const featured = query ? [] : blogPosts.filter((post) => post.featured).slice(0, 2);
  const categories = getCategories();
  const tags = getTags();

  return (
    <main className={styles.page}>
      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <span className={styles.eyebrow}>CodeLocal Journal</span>
          <h1>Build AI agents that can act without losing control.</h1>
          <p>
            Engineering notes, product thinking and practical guides for local-first AI,
            MCP, project intelligence and safer agent workflows.
          </p>
        </div>
        <aside className={styles.heroAside}>
          <strong>Find what you need</strong>
          <p>Search across titles, summaries, categories and tags.</p>
          <form className={styles.searchForm} action="/blog" method="get">
            <label className="sr-only" htmlFor="blog-search">Search the CodeLocal Blog</label>
            <input
              id="blog-search"
              name="q"
              type="search"
              defaultValue={query}
              placeholder="MCP, security, Project Brain…"
            />
            <button type="submit">Search</button>
          </form>
        </aside>
      </section>

      {featured.length > 0 && (
        <section className={styles.section} aria-labelledby="featured-posts">
          <div className={styles.sectionHead}>
            <div>
              <span className={styles.sectionLabel}>Featured</span>
              <h2 id="featured-posts">Start here</h2>
            </div>
            <p>Selected product and engineering notes</p>
          </div>
          <div className={styles.postGrid}>
            {featured.map((post) => <PostCard key={post.slug} post={post} />)}
          </div>
        </section>
      )}

      <section className={styles.section} aria-labelledby="latest-posts">
        <div className={styles.sectionHead}>
          <div>
            <span className={styles.sectionLabel}>{query ? "Search" : "Latest"}</span>
            <h2 id="latest-posts">{query ? `Results for “${query}”` : "All articles"}</h2>
          </div>
          <p>{posts.length} {posts.length === 1 ? "article" : "articles"}</p>
        </div>
        {posts.length > 0 ? (
          <div className={styles.postGrid}>
            {posts.map((post) => <PostCard key={post.slug} post={post} compact />)}
          </div>
        ) : <EmptyState query={query} />}
      </section>

      {!query && (
        <>
          <section className={styles.section} aria-labelledby="blog-series">
            <div className={styles.sectionHead}>
              <div>
                <span className={styles.sectionLabel}>Learn in order</span>
                <h2 id="blog-series">Series</h2>
              </div>
              <Link className={styles.readMore} href="/blog/series">View all series <span aria-hidden="true">→</span></Link>
            </div>
            <div className={styles.seriesGrid}>
              {blogSeries.map((series) => <SeriesCard key={series.slug} series={series} />)}
            </div>
          </section>

          <section className={styles.section} aria-labelledby="explore-blog">
            <div className={styles.sectionHead}>
              <div>
                <span className={styles.sectionLabel}>Explore</span>
                <h2 id="explore-blog">Topics</h2>
              </div>
              <p>Browse by category or tag</p>
            </div>
            <div className={styles.taxonomyCloud}>
              {categories.map((category) => (
                <Link key={category} href={`/blog/category/${taxonomySlug(category)}`}>{category}</Link>
              ))}
              {tags.map((tag) => (
                <Link key={tag} href={`/blog/tag/${taxonomySlug(tag)}`}>#{tag}</Link>
              ))}
            </div>
          </section>
        </>
      )}
    </main>
  );
}
