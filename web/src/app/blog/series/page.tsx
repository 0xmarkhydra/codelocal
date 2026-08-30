import { SeriesCard } from "../_components";
import styles from "../blog.module.css";
import { blogSeries } from "@/lib/blog";

export default function SeriesIndexPage() {
  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>CodeLocal Series</span>
        <h1>Learn a topic from first principles.</h1>
        <p>
          Series group related articles into an ordered path. Start at part one, jump to the
          piece you need, or follow progress directly from every article.
        </p>
      </header>

      <section className={styles.section} aria-labelledby="all-series">
        <div className={styles.sectionHead}>
          <div>
            <span className={styles.sectionLabel}>Collections</span>
            <h2 id="all-series">All series</h2>
          </div>
          <p>{blogSeries.length} {blogSeries.length === 1 ? "series" : "series"}</p>
        </div>
        <div className={styles.seriesGrid}>
          {blogSeries.map((series) => <SeriesCard key={series.slug} series={series} />)}
        </div>
      </section>
    </main>
  );
}
