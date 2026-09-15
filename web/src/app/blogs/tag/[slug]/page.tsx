import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../../blog/_components";
import styles from "../../../blog/blog.module.css";
import { getTags, taxonomySlug } from "@/lib/blog";
import { getTranslations } from "@/lib/i18n/server";
import { decodeBlogRouteSlug, getBlogTagsForRender, getPostsByTagSlugForRender } from "@/lib/blog-server";

type TagPageProps = { params: Promise<{ slug: string }> };

export function generateStaticParams() {
  return getTags().map((tag) => ({ slug: taxonomySlug(tag) }));
}

async function resolveTag(slug: string) {
  return (await getBlogTagsForRender()).find((tag) => taxonomySlug(tag) === slug);
}

export async function generateMetadata({ params }: TagPageProps): Promise<Metadata> {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const tag = await resolveTag(slug);
  if (!tag) return { title: t("Tag not found"), robots: { index: false, follow: false } };
  return {
    title: `#${tag}`,
    description: t("CodeLocal Blog articles tagged {tag}.", { tag }),
    alternates: { canonical: `/blogs/tag/${slug}` },
  };
}

export default async function TagPage({ params }: TagPageProps) {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const [tag, posts] = await Promise.all([resolveTag(slug), getPostsByTagSlugForRender(slug)]);
  if (!tag) notFound();

  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>{t("Tag")}</span>
        <h1>#{tag}</h1>
      </header>
      <section className={styles.section} aria-labelledby="tagged-articles">
        <div className={styles.sectionHead}>
          <h2 id="tagged-articles">{t("Tagged articles")}</h2>
          <p>{t("{count} articles", { count: posts.length })}</p>
        </div>
        <div className={styles.postGrid}>{posts.map((post) => <PostCard key={post.slug} post={post} compact />)}</div>
      </section>
    </main>
  );
}
