import Image from "next/image";
import Link from "next/link";
import type { BlogBlock, BlogPost, BlogSeries } from "@/lib/blog";
import { formatBlogDate, getSeriesPosts, taxonomySlug } from "@/lib/blog";
import { getLocale, getTranslations } from "@/lib/i18n/server";
import { LanguageSelect } from "@/lib/i18n/provider";
import styles from "./blog.module.css";

export async function BlogHeader() {
  const t = await getTranslations();
  return (
    <header className={styles.siteHeader}>
      <Link className={styles.brand} href="/" aria-label={t("CodeLocal home")}>
        <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
        <span>CodeLocal</span>
        <em>{t("Journal")}</em>
      </Link>
      <nav className={styles.headerNav} aria-label={t("Blog navigation")}>
        <Link href="/blogs">{t("Latest")}</Link>
        <Link href="/blogs/series">{t("Series")}</Link>
        <Link href="/security">{t("Security")}</Link>
      </nav>
      <LanguageSelect />
      <Link className={styles.openApp} href="/dashboard">{t("Open app")}</Link>
    </header>
  );
}

export async function BlogFooter() {
  const t = await getTranslations();
  return (
    <footer className={styles.siteFooter}>
      <div>
        <strong>{t("CodeLocal Blog")}</strong>
        <p>{t("Practical notes on local-first AI infrastructure, agents and project intelligence.")}</p>
      </div>
      <nav aria-label={t("Footer navigation")}>
        <Link href="/">{t("Product")}</Link>
        <Link href="/security">{t("Security")}</Link>
        <Link href="/support">{t("Support")}</Link>
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

export async function PostCard({ post, compact = false }: { post: BlogPost; compact?: boolean }) {
  const t = await getTranslations();
  const locale = await getLocale();
  return (
    <article className={`${styles.postCard} ${compact ? styles.postCardCompact : ""}`}>
      <div className={styles.postCardMeta}>
        {post.series ? <span>{t("Part {count}", { count: post.series.part })}</span> : <span>{t("Standalone")}</span>}
        <time dateTime={post.publishedAt}>{formatBlogDate(post.publishedAt, locale)}</time>
        <span>{t("{count} min read", { count: post.readingMinutes })}</span>
      </div>
      <h2><Link href={`/blogs/${post.slug}`}>{post.title}</Link></h2>
      <p>{post.excerpt}</p>
      <TaxonomyLinks post={post} />
      <Link className={styles.readMore} href={`/blogs/${post.slug}`}>{t("Read article")} <span aria-hidden="true">→</span></Link>
    </article>
  );
}

export async function ArticleAbout({ post, seriesTitle }: { post: BlogPost; seriesTitle?: string }) {
  const t = await getTranslations();
  const locale = await getLocale();
  const monogram = new Intl.Segmenter(locale, { granularity: "grapheme" }).segment(post.author.name.trim())[Symbol.iterator]().next().value?.segment.toLocaleUpperCase(locale) || "C";
  const description = post.series && seriesTitle
    ? t("Part {count} in {series}.", { count: post.series.part, series: seriesTitle })
    : post.official
      ? t("An original article from the CodeLocal team.")
      : t("An article from the CodeLocal community.");

  return (
    <div className={`${styles.asideCard} ${styles.aboutCard}`}>
      <div className={styles.aboutTopline}>
        <span>{t("About this article")}</span>
        <span className={styles.aboutBadge}>{t(post.official ? "Original" : "Community")}</span>
      </div>
      <Link className={styles.aboutCategory} href={`/blogs/category/${taxonomySlug(post.category)}`}>{post.category}</Link>
      <p>{description}</p>
      <dl className={styles.aboutMeta}>
        <div>
          <dt>{t("Published")}</dt>
          <dd><time dateTime={post.publishedAt}>{formatBlogDate(post.publishedAt, locale)}</time></dd>
        </div>
        <div>
          <dt>{t("Reading time")}</dt>
          <dd>{t("{count} min read", { count: post.readingMinutes })}</dd>
        </div>
      </dl>
      <div className={styles.aboutAuthor}>
        <span className={styles.authorMonogram} aria-hidden="true">{monogram}</span>
        <div>
          <span>{t("Written by")}</span>
          <Link href={`/users/${post.author.slug}`}>{post.author.name}</Link>
          <small>{post.author.role}</small>
        </div>
      </div>
    </div>
  );
}

export async function RelatedReading({ posts }: { posts: BlogPost[] }) {
  if (posts.length === 0) return null;
  const t = await getTranslations();

  return (
    <section className={styles.related} aria-labelledby="related-reading">
      <header className={styles.relatedHeader}>
        <div>
          <h2 id="related-reading">{t("Keep reading")}</h2>
        </div>
        <Link className={styles.relatedBrowse} href="/blogs">{t("View all articles")} <span aria-hidden="true">↗</span></Link>
      </header>
      <div className={`${styles.postGrid} ${styles.relatedGrid}`}>
        {posts.map((post) => <PostCard key={post.slug} post={post} compact />)}
      </div>
    </section>
  );
}

export async function SeriesCard({ series, posts: suppliedPosts }: { series: BlogSeries; posts?: BlogPost[] }) {
  const t = await getTranslations();
  const locale = await getLocale();
  const posts = suppliedPosts ?? getSeriesPosts(series.slug);
  return (
    <article className={styles.seriesCard}>
      <div className={styles.seriesCardTop}>
        <span>{t(series.status === "complete" ? "Complete series" : "Active series")}</span>
        <em>{t("{count} parts", { count: posts.length })}</em>
      </div>
      <h2><Link href={`/blogs/series/${series.slug}`}>{series.title}</Link></h2>
      <p>{series.description}</p>
      {posts.length > 0 && (
        <ol>
          {posts.map((post) => (
            <li key={post.slug}>
              <span>{new Intl.NumberFormat(locale, { minimumIntegerDigits: 2 }).format(post.series?.part ?? 0)}</span>
              <Link href={`/blogs/${post.slug}`}>{post.title}</Link>
            </li>
          ))}
        </ol>
      )}
      <Link className={styles.readMore} href={`/blogs/series/${series.slug}`}>{t("Explore series")} <span aria-hidden="true">→</span></Link>
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

export async function EmptyState({ query }: { query?: string }) {
  const t = await getTranslations();
  return (
    <div className={styles.emptyState}>
      <strong>{t("No articles found.")}</strong>
      <p>{query ? t("Nothing matched “{query}”. Try a broader search.", { query }) : t("There are no published articles in this view yet.")}</p>
      <Link href="/blogs">{t("View all articles")}</Link>
    </div>
  );
}
