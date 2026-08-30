import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../_components";
import styles from "../../blog.module.css";
import { blogSeries, getSeriesBySlug, getSeriesPosts } from "@/lib/blog";

type SeriesPageProps = { params: Promise<{ slug: string }> };

export function generateStaticParams() {
  return blogSeries.map((series) => ({ slug: series.slug }));
}

export async function generateMetadata({ params }: SeriesPageProps): Promise<Metadata> {
  const { slug } = await params;
  const series = getSeriesBySlug(slug);
  if (!series) return { title: "Series not found" };
  return { title: series.title, description: series.description };
}

export default async function SeriesPage({ params }: SeriesPageProps) {
  const { slug } = await params;
  const series = getSeriesBySlug(slug);
  if (!series) notFound();
  const posts = getSeriesPosts(series.slug);

  return (
    <main className={styles.page}>
      <header className={styles.seriesHeader}>
        <span className={styles.eyebrow}>{series.status === "complete" ? "Complete series" : "Active series"}</span>
        <h1>{series.title}</h1>
        <p>{series.description}</p>
        <div className={styles.articleMeta}>
          <strong>{series.category}</strong>
          <span>{posts.length} parts</span>
          <span>{posts.reduce((total, post) => total + post.readingMinutes, 0)} min total</span>
        </div>
      </header>

      <section className={styles.section} aria-labelledby="series-parts">
        <div className={styles.sectionHead}>
          <div>
            <span className={styles.sectionLabel}>Reading path</span>
            <h2 id="series-parts">In this series</h2>
          </div>
          <p>Read in order or jump to any part</p>
        </div>
        <div className={styles.postGrid}>
          {posts.map((post) => <PostCard key={post.slug} post={post} compact />)}
        </div>
      </section>
    </main>
  );
}
