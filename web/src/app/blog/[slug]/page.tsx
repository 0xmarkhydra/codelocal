import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { BlogBlocks, PostCard, TaxonomyLinks } from "../_components";
import styles from "../blog.module.css";
import {
  blogPosts,
  formatBlogDate,
  getRelatedPosts,
  getSeriesBySlug,
  getSeriesPosts,
} from "@/lib/blog";
import { getBlogPostForRender } from "@/lib/blog-server";

type ArticleProps = { params: Promise<{ slug: string }> };

function publicCoverURL(assetID: string) {
  return `/api/v1/public/media/${encodeURIComponent(assetID)}/large`;
}

export function generateStaticParams() {
  return blogPosts.map((post) => ({ slug: post.slug }));
}

export async function generateMetadata({ params }: ArticleProps): Promise<Metadata> {
  const { slug } = await params;
  const post = await getBlogPostForRender(slug);
  if (!post) return { title: "Article not found" };

  return {
    title: post.title,
    description: post.excerpt,
    openGraph: {
      type: "article",
      title: post.title,
      description: post.excerpt,
      publishedTime: `${post.publishedAt}T00:00:00Z`,
      modifiedTime: post.updatedAt ? `${post.updatedAt}T00:00:00Z` : undefined,
      tags: post.tags,
      images: post.coverAssetId ? [{ url: publicCoverURL(post.coverAssetId) }] : undefined,
    },
  };
}

export default async function ArticlePage({ params }: ArticleProps) {
  const { slug } = await params;
  const post = await getBlogPostForRender(slug);
  if (!post) notFound();

  const series = post.series ? getSeriesBySlug(post.series.slug) : undefined;
  const seriesPosts = series ? getSeriesPosts(series.slug) : [];
  const seriesIndex = seriesPosts.findIndex((entry) => entry.slug === post.slug);
  const previous = seriesIndex > 0 ? seriesPosts[seriesIndex - 1] : undefined;
  const next = seriesIndex >= 0 ? seriesPosts[seriesIndex + 1] : undefined;
  const related = getRelatedPosts(post);
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    headline: post.title,
    description: post.excerpt,
    datePublished: post.publishedAt,
    dateModified: post.updatedAt ?? post.publishedAt,
    author: { "@type": "Person", name: post.author.name },
    publisher: { "@type": "Organization", name: "CodeLocal" },
    mainEntityOfPage: `https://codelocal.cloud/blog/${post.slug}`,
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
        <span className={styles.eyebrow}>{series ? `Series · Part ${post.series?.part}` : "Standalone article"}</span>
        <h1>{post.title}</h1>
        <p>{post.excerpt}</p>
        <div className={styles.articleMeta}>
          <strong>{post.author.name}</strong>
          <span>{post.author.role}</span>
          <time dateTime={post.publishedAt}>{formatBlogDate(post.publishedAt)}</time>
          <span>{post.readingMinutes} min read</span>
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
            <span>Part {post.series?.part} of {seriesPosts.length}</span>
            <strong>{series.title}</strong>
          </div>
          <Link href={`/blog/series/${series.slug}`}>View full series →</Link>
        </div>
      )}

      <div className={styles.articleLayout}>
        <article>
          <TaxonomyLinks post={post} />
          <BlogBlocks blocks={post.blocks} />
          {series && (previous || next) && (
            <nav className={styles.prevNext} aria-label="Series navigation">
              {previous ? (
                <Link href={`/blog/${previous.slug}`}>
                  <span>← Previous part</span>
                  <strong>{previous.title}</strong>
                </Link>
              ) : <span />}
              {next ? (
                <Link href={`/blog/${next.slug}`}>
                  <span>Next part →</span>
                  <strong>{next.title}</strong>
                </Link>
              ) : <span />}
            </nav>
          )}
        </article>

        <aside className={styles.articleAside}>
          <div className={styles.asideCard}>
            <span>About this article</span>
            <strong>{post.category}</strong>
            <p>{post.series ? `Part ${post.series.part} in ${series?.title}.` : "A standalone CodeLocal article."}</p>
          </div>
          {series && (
            <div className={styles.asideCard}>
              <span>Series progress</span>
              <strong>{series.title}</strong>
              <ol className={styles.seriesProgress}>
                {seriesPosts.map((entry) => (
                  <li key={entry.slug} data-current={entry.slug === post.slug}>
                    <Link href={`/blog/${entry.slug}`}>
                      <span>{String(entry.series?.part ?? 0).padStart(2, "0")}</span>
                      {entry.title}
                    </Link>
                  </li>
                ))}
              </ol>
            </div>
          )}
        </aside>
      </div>

      {related.length > 0 && (
        <section className={styles.related} aria-labelledby="related-reading">
          <h2 id="related-reading">Keep reading</h2>
          <div className={styles.postGrid}>
            {related.map((entry) => <PostCard key={entry.slug} post={entry} compact />)}
          </div>
        </section>
      )}
    </main>
  );
}
