import { SeriesCard } from "../../blog/_components";
import styles from "../../blog/blog.module.css";
import { getBlogPostsForRender, getBlogSeriesForRender } from "@/lib/blog-server";
import { getTranslations } from "@/lib/i18n/server";

export async function generateMetadata() {
  const t = await getTranslations();
  return { title: t("CodeLocal Series"), alternates: { canonical: "/blogs/series" } };
}

export default async function SeriesIndexPage() {
  const t = await getTranslations();
  const [series, posts] = await Promise.all([getBlogSeriesForRender(), getBlogPostsForRender()]);
  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <h1>{t("CodeLocal Series")}</h1>
      </header>

      <section className={styles.section} aria-labelledby="all-series">
        <div className={styles.sectionHead}>
          <h2 id="all-series">{t("All series")}</h2>
          <p>{t("{count} series", { count: series.length })}</p>
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
