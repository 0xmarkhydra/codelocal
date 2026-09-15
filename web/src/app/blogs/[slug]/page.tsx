import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound, permanentRedirect } from "next/navigation";
import { ArticleAbout, BlogBlocks, RelatedReading, TaxonomyLinks } from "../../blog/_components";
import styles from "../../blog/blog.module.css";
import { blogPosts, formatBlogDate } from "@/lib/blog";
import { getLocale, getTranslations } from "@/lib/i18n/server";
import {
  decodeBlogRouteSlug,
  getBlogPostForRender,
  getBlogSeriesPageForRender,
  getRelatedPostsForRender,
} from "@/lib/blog-server";

type ArticleProps = { params: Promise<{ slug: string }> };

function publicCoverURL(assetID: string) {
  return `/api/v1/public/media/${encodeURIComponent(assetID)}/large`;
}

export function generateStaticParams() {
  return blogPosts.map((post) => ({ slug: post.slug }));
}

export async function generateMetadata({ params }: ArticleProps): Promise<Metadata> {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const post = await getBlogPostForRender(slug);
  if (!post) return { title: t("Article not found"), robots: { index: false, follow: false } };

  return {
    title: post.title,
    description: post.excerpt,
    alternates: { canonical: `/blogs/${post.slug}` },
    openGraph: {
      type: "article",
      title: post.title,
      description: post.excerpt,
      url: `/blogs/${post.slug}`,
      publishedTime: `${post.publishedAt}T00:00:00Z`,
      modifiedTime: post.updatedAt ? `${post.updatedAt}T00:00:00Z` : undefined,
      tags: post.tags,
      images: post.coverAssetId ? [{ url: publicCoverURL(post.coverAssetId) }] : undefined,
    },
  };
}

export default async function ArticlePage({ params }: ArticleProps) {
  const t = await getTranslations();
  const locale = await getLocale();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const post = await getBlogPostForRender(slug);
  if (!post) notFound();
  if (slug !== post.slug) permanentRedirect(`/blogs/${post.slug}`);

  const [seriesData, related] = await Promise.all([
    post.series ? getBlogSeriesPageForRender(post.series.slug) : Promise.resolve(undefined),
    getRelatedPostsForRender(post),
  ]);
  const series = seriesData?.series;
  const seriesPosts = seriesData?.posts ?? [];
  const seriesIndex = seriesPosts.findIndex((entry) => entry.slug === post.slug);
  const previous = seriesIndex > 0 ? seriesPosts[seriesIndex - 1] : undefined;
  const next = seriesIndex >= 0 ? seriesPosts[seriesIndex + 1] : undefined;
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    headline: post.title,
    description: post.excerpt,
    datePublished: post.publishedAt,
    dateModified: post.updatedAt ?? post.publishedAt,
    author: { "@type": "Person", name: post.author.name, url: `https://codelocal.cloud/users/${post.author.slug}` },
    publisher: { "@type": "Organization", name: "CodeLocal" },
    mainEntityOfPage: `https://codelocal.cloud/blogs/${post.slug}`,
    image: post.coverAssetId ? `https://codelocal.cloud${publicCoverURL(post.coverAssetId)}` : undefined,
    keywords: post.tags.join(", "),
  };

  return (
    <main className={styles.articlePage}>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }}
      />

      <header className={styles.articleHeader}>
        <span className={styles.eyebrow}>{series ? t("Series · Part {count}", { count: post.series?.part ?? 0 }) : t("Standalone article")}</span>
        <h1>{post.title}</h1>
        <p>{post.excerpt}</p>
        <div className={styles.articleMeta}>
          <Link href={`/users/${post.author.slug}`}><strong>{post.author.name}</strong></Link>
          <span>{post.author.role}</span>
          <time dateTime={post.publishedAt}>{formatBlogDate(post.publishedAt, locale)}</time>
          <span>{t("{count} min read", { count: post.readingMinutes })}</span>
        </div>
      </header>

      {post.coverAssetId && (
        <div className={styles.articleCover}>
          <Image
            src={publicCoverURL(post.coverAssetId)}
            alt=""
            width={1600}
            height={900}
            sizes="(max-width: 1160px) 100vw, 1160px"
            priority
            unoptimized
          />
        </div>
      )}

      {series && (
        <div className={styles.seriesBanner}>
          <div>
            <span>{t("Part {part} of {count}", { part: post.series?.part ?? 0, count: seriesPosts.length })}</span>
            <strong>{series.title}</strong>
          </div>
          <Link href={`/blogs/series/${series.slug}`}>{t("View full series")} <span aria-hidden="true">→</span></Link>
        </div>
      )}

      <div className={styles.articleLayout}>
        <article>
          <TaxonomyLinks post={post} />
          <BlogBlocks blocks={post.blocks} />
          {series && (previous || next) && (
            <nav className={styles.prevNext} aria-label={t("Series navigation")}>
              {previous ? (
                <Link href={`/blogs/${previous.slug}`}>
                  <span><span aria-hidden="true">← </span>{t("Previous part")}</span>
                  <strong>{previous.title}</strong>
                </Link>
              ) : <span />}
              {next ? (
                <Link href={`/blogs/${next.slug}`}>
                  <span>{t("Next part")}<span aria-hidden="true"> →</span></span>
                  <strong>{next.title}</strong>
                </Link>
              ) : <span />}
            </nav>
          )}
        </article>

        <aside className={styles.articleAside}>
          <ArticleAbout post={post} seriesTitle={series?.title} />
          {series && (
            <div className={styles.asideCard}>
              <span>{t("Series progress")}</span>
              <strong>{series.title}</strong>
              <ol className={styles.seriesProgress}>
                {seriesPosts.map((entry) => (
                  <li key={entry.slug} data-current={entry.slug === post.slug}>
                    <Link href={`/blogs/${entry.slug}`} aria-current={entry.slug === post.slug ? "page" : undefined}>
                      <span>{new Intl.NumberFormat(locale, { minimumIntegerDigits: 2 }).format(entry.series?.part ?? 0)}</span>
                      {entry.title}
                    </Link>
                  </li>
                ))}
              </ol>
            </div>
          )}
        </aside>
      </div>

      <RelatedReading posts={related} />
    </main>
  );
}
