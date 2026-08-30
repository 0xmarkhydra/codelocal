import { SeriesCard } from "../../blog/_components";
import styles from "../../blog/blog.module.css";
import { getBlogPostsForRender, getBlogSeriesForRender } from "@/lib/blog-server";

export default async function SeriesIndexPage() {
  const [series, posts] = await Promise.all([getBlogSeriesForRender(), getBlogPostsForRender()]);
  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>CodeLocal Series</span>
        <h1>Learn a topic from first principles.</h1>
        <p>Series group related articles into an ordered path from CodeLocal and community authors.</p>
      </header>

      <section className={styles.section} aria-labelledby="all-series">
        <div className={styles.sectionHead}>
          <div><span className={styles.sectionLabel}>Collections</span><h2 id="all-series">All series</h2></div>
          <p>{series.length} {series.length === 1 ? "series" : "series"}</p>
        </div>
        <div className={styles.seriesGrid}>
          {series.map((item) => (
            <SeriesCard key={item.slug} series={item} posts={posts.filter((post) => post.series?.slug === item.slug)} />
          ))}
        </div>
      </section>
    </main>
  );
}
