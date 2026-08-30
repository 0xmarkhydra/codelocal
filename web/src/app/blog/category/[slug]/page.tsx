import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../_components";
import styles from "../../blog.module.css";
import { getCategories, getPostsByCategorySlug, taxonomySlug } from "@/lib/blog";

type CategoryPageProps = { params: Promise<{ slug: string }> };

export function generateStaticParams() {
  return getCategories().map((category) => ({ slug: taxonomySlug(category) }));
}

export async function generateMetadata({ params }: CategoryPageProps): Promise<Metadata> {
  const { slug } = await params;
  const posts = getPostsByCategorySlug(slug);
  const category = posts[0]?.category;
  if (!category) return { title: "Category not found" };
  return {
    title: `${category} articles`,
    description: `CodeLocal Blog articles filed under ${category}.`,
  };
}

export default async function CategoryPage({ params }: CategoryPageProps) {
  const { slug } = await params;
  const posts = getPostsByCategorySlug(slug);
  if (posts.length === 0) notFound();
  const category = posts[0].category;

  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>Category</span>
        <h1>{category}</h1>
        <p>Articles, guides and product notes grouped under {category}.</p>
      </header>

      <section className={styles.section} aria-labelledby="category-articles">
        <div className={styles.sectionHead}>
          <div>
            <span className={styles.sectionLabel}>Archive</span>
            <h2 id="category-articles">Articles</h2>
          </div>
          <p>{posts.length} {posts.length === 1 ? "article" : "articles"}</p>
        </div>
        <div className={styles.postGrid}>
          {posts.map((post) => <PostCard key={post.slug} post={post} compact />)}
        </div>
      </section>
    </main>
  );
}
