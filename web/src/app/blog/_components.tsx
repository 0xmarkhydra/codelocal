import Image from "next/image";
import Link from "next/link";
import type { BlogBlock, BlogPost, BlogSeries } from "@/lib/blog";
import { formatBlogDate, getSeriesPosts, taxonomySlug } from "@/lib/blog";
import styles from "./blog.module.css";

export function BlogHeader() {
  return (
    <header className={styles.siteHeader}>
      <Link className={styles.brand} href="/" aria-label="CodeLocal home">
        <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
        <span>CodeLocal</span>
        <em>Blog</em>
      </Link>
      <nav className={styles.headerNav} aria-label="Blog navigation">
        <Link href="/blogs">Latest</Link>
        <Link href="/blogs/series">Series</Link>
        <Link href="/security">Security</Link>
      </nav>
      <Link className={styles.openApp} href="/dashboard">Open app</Link>
    </header>
  );
}

export function BlogFooter() {
  return (
    <footer className={styles.siteFooter}>
      <div>
        <strong>CodeLocal Blog</strong>
        <p>Practical notes on local-first AI infrastructure, agents and project intelligence.</p>
      </div>
      <nav aria-label="Footer navigation">
        <Link href="/">Product</Link>
        <Link href="/security">Security</Link>
        <Link href="/support">Support</Link>
      </nav>
    </footer>
  );
}

export function TaxonomyLinks({ post }: { post: BlogPost }) {
  return (
    <div className={styles.taxonomy}>
      <Link href={`/blogs/category/${taxonomySlug(post.category)}`}>{post.category}</Link>
      {post.tags.map((tag) => (
        <Link key={tag} href={`/blogs/tag/${taxonomySlug(tag)}`}>#{tag}</Link>
      ))}
    </div>
  );
}

export function PostCard({ post, compact = false }: { post: BlogPost; compact?: boolean }) {
  return (
    <article className={`${styles.postCard} ${compact ? styles.postCardCompact : ""}`}>
      <div className={styles.postCardMeta}>
        {post.series ? <span>Part {post.series.part}</span> : <span>Standalone</span>}
        <time dateTime={post.publishedAt}>{formatBlogDate(post.publishedAt)}</time>
        <span>{post.readingMinutes} min</span>
      </div>
      <h2><Link href={`/blogs/${post.slug}`}>{post.title}</Link></h2>
      <p>{post.excerpt}</p>
      <TaxonomyLinks post={post} />
      <Link className={styles.readMore} href={`/blogs/${post.slug}`}>Read article <span aria-hidden="true">→</span></Link>
    </article>
  );
}

export function SeriesCard({ series, posts: suppliedPosts }: { series: BlogSeries; posts?: BlogPost[] }) {
  const posts = suppliedPosts ?? getSeriesPosts(series.slug);
  return (
    <article className={styles.seriesCard}>
      <div className={styles.seriesCardTop}>
        <span>{series.status === "complete" ? "Complete series" : "Active series"}</span>
        <em>{posts.length} parts</em>
      </div>
      <h2><Link href={`/blogs/series/${series.slug}`}>{series.title}</Link></h2>
      <p>{series.description}</p>
      {posts.length > 0 && (
        <ol>
          {posts.map((post) => (
            <li key={post.slug}>
              <span>{String(post.series?.part ?? 0).padStart(2, "0")}</span>
              <Link href={`/blogs/${post.slug}`}>{post.title}</Link>
            </li>
          ))}
        </ol>
      )}
      <Link className={styles.readMore} href={`/blogs/series/${series.slug}`}>Explore series <span aria-hidden="true">→</span></Link>
    </article>
  );
}

export function BlogBlocks({ blocks }: { blocks: BlogBlock[] }) {
  return (
    <div className={styles.articleBody}>
      {blocks.map((block, index) => {
        const key = `${block.type}-${index}`;
        if (block.type === "heading") return <h2 key={key}>{block.text}</h2>;
        if (block.type === "paragraph") return <p key={key}>{block.text}</p>;
        if (block.type === "list") {
          return <ul key={key}>{block.items.map((item) => <li key={item}>{item}</li>)}</ul>;
        }
        if (block.type === "code") {
          return (
            <pre key={key} data-language={block.language ?? "text"}>
              <code>{block.code}</code>
            </pre>
          );
        }
        if (block.type === "image") {
          return (
            <figure className={styles.articleImage} key={key}>
              <Image
                src={`/api/v1/public/media/${encodeURIComponent(block.assetId)}/${block.variant ?? "large"}`}
                alt={block.alt ?? ""}
                width={block.width}
                height={block.height}
                sizes="(max-width: 860px) 100vw, 760px"
                unoptimized
              />
              {block.caption && <figcaption>{block.caption}</figcaption>}
            </figure>
          );
        }
        return (
          <aside className={styles.callout} key={key}>
            <strong>{block.title}</strong>
            <p>{block.text}</p>
          </aside>
        );
      })}
    </div>
  );
}

export function EmptyState({ query }: { query?: string }) {
  return (
    <div className={styles.emptyState}>
      <strong>No articles found.</strong>
      <p>{query ? `Nothing matched “${query}”. Try a broader search.` : "There are no published articles in this view yet."}</p>
      <Link href="/blogs">View all articles</Link>
    </div>
  );
}
