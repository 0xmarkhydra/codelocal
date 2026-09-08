import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../../blog/_components";
import styles from "../../../blog/blog.module.css";
import { getCategories, taxonomySlug } from "@/lib/blog";
import { getTranslations } from "@/lib/i18n/server";
import { decodeBlogRouteSlug, getPostsByCategorySlugForRender } from "@/lib/blog-server";

type CategoryPageProps = { params: Promise<{ slug: string }> };

export function generateStaticParams() {
  return getCategories().map((category) => ({ slug: taxonomySlug(category) }));
}

export async function generateMetadata({ params }: CategoryPageProps): Promise<Metadata> {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const posts = await getPostsByCategorySlugForRender(slug);
  const category = posts[0]?.category;
  if (!category) return { title: t("Category not found"), robots: { index: false, follow: false } };
  return {
    title: t("{category} articles", { category }),
    description: t("CodeLocal Blog articles filed under {category}.", { category }),
    alternates: { canonical: `/blogs/category/${slug}` },
  };
}

export default async function CategoryPage({ params }: CategoryPageProps) {
  const t = await getTranslations();
  const { slug: routeSlug } = await params;
  const slug = decodeBlogRouteSlug(routeSlug);
  const posts = await getPostsByCategorySlugForRender(slug);
  if (posts.length === 0) notFound();
  const category = posts[0].category;

  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>{t("Category")}</span>
        <h1>{category}</h1>
      </header>
      <section className={styles.section} aria-labelledby="category-articles">
        <div className={styles.sectionHead}>
          <h2 id="category-articles">{t("Articles")}</h2>
          <p>{t("{count} articles", { count: posts.length })}</p>
        </div>
        <div className={styles.postGrid}>{posts.map((post) => <PostCard key={post.slug} post={post} compact />)}</div>
      </section>
    </main>
  );
}
