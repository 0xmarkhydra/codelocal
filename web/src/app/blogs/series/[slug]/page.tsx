import type { Metadata } from "next";
import Image from "next/image";
import { notFound, permanentRedirect } from "next/navigation";
import { PostCard } from "../../../blog/_components";
import styles from "../../../blog/blog.module.css";
import { blogSeries } from "@/lib/blog";
import { getTranslations } from "@/lib/i18n/server";
import { decodeBlogRouteSlug, getBlogSeriesPageForRender } from "@/lib/blog-server";

type SeriesPageProps = { params: Promise<{ slug: string }> };

function publicCoverURL(assetID: string) {
  return `/api/v1/public/media/${encodeURIComponent(assetID)}/large`;
}

export function generateStaticParams() {
  return blogSeries.map((series) => ({ slug: series.slug }));
}

export async function generateMetadata({ params }: SeriesPageProps): Promise<Metadata> {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const result = await getBlogSeriesPageForRender(slug);
  if (!result) return { title: t("Series not found"), robots: { index: false, follow: false } };
  return {
    title: result.series.title,
    description: result.series.description,
    alternates: { canonical: `/blogs/series/${result.series.slug}` },
    openGraph: {
      title: result.series.title,
      description: result.series.description,
      url: `/blogs/series/${result.series.slug}`,
      images: result.series.coverAssetId ? [{ url: publicCoverURL(result.series.coverAssetId) }] : undefined,
    },
  };
}

export default async function SeriesPage({ params }: SeriesPageProps) {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const result = await getBlogSeriesPageForRender(slug);
  if (!result) notFound();
  if (result.redirected || slug !== result.series.slug) permanentRedirect(`/blogs/series/${result.series.slug}`);
  const { series, posts } = result;

  return (
    <main className={styles.page}>
      <header className={styles.seriesHeader}>
        <span className={styles.eyebrow}>{t(series.status === "complete" ? "Complete series" : "Active series")}</span>
        <h1>{series.title}</h1>
        <p>{series.description}</p>
        <div className={styles.articleMeta}>
          {series.author && <strong>{series.author.name}</strong>}
          <span>{t("{count} parts", { count: posts.length })}</span>
          <span>{t("{count} min total", { count: posts.reduce((total, post) => total + post.readingMinutes, 0) })}</span>
        </div>
      </header>

      {series.coverAssetId && (
        <div className={styles.articleCover}>
          <Image src={publicCoverURL(series.coverAssetId)} alt="" width={1600} height={900} sizes="(max-width: 1160px) 100vw, 1160px" priority unoptimized />
        </div>
      )}

      <section className={styles.section} aria-labelledby="series-parts">
        <div className={styles.sectionHead}>
          <h2 id="series-parts">{t("In this series")}</h2>
        </div>
        <div className={styles.postGrid}>
          {posts.map((post) => <PostCard key={post.slug} post={post} compact />)}
        </div>
      </section>
    </main>
  );
}
