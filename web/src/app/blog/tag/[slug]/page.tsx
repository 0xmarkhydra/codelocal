import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../_components";
import styles from "../../blog.module.css";
import { getPostsByTagSlug, getTags, taxonomySlug } from "@/lib/blog";

type TagPageProps = { params: Promise<{ slug: string }> };

function resolveTag(slug: string) {
  return getTags().find((tag) => taxonomySlug(tag) === slug);
}

export function generateStaticParams() {
  return getTags().map((tag) => ({ slug: taxonomySlug(tag) }));
}

export async function generateMetadata({ params }: TagPageProps): Promise<Metadata> {
  const { slug } = await params;
  const tag = resolveTag(slug);
  if (!tag) return { title: "Tag not found" };
  return {
    title: `#${tag}`,
    description: `CodeLocal Blog articles tagged ${tag}.`,
  };
}

export default async function TagPage({ params }: TagPageProps) {
  const { slug } = await params;
  const tag = resolveTag(slug);
  if (!tag) notFound();
  const posts = getPostsByTagSlug(slug);

  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>Tag</span>
        <h1>#{tag}</h1>
        <p>Every CodeLocal Blog article connected to {tag}.</p>
      </header>

      <section className={styles.section} aria-labelledby="tagged-articles">
        <div className={styles.sectionHead}>
          <div>
            <span className={styles.sectionLabel}>Archive</span>
            <h2 id="tagged-articles">Tagged articles</h2>
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
