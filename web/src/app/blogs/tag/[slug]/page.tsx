import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../../blog/_components";
import styles from "../../../blog/blog.module.css";
import { getTags, taxonomySlug } from "@/lib/blog";
import { getBlogTagsForRender, getPostsByTagSlugForRender } from "@/lib/blog-server";

type TagPageProps = { params: Promise<{ slug: string }> };

export function generateStaticParams() {
  return getTags().map((tag) => ({ slug: taxonomySlug(tag) }));
}

async function resolveTag(slug: string) {
  return (await getBlogTagsForRender()).find((tag) => taxonomySlug(tag) === slug);
}

export async function generateMetadata({ params }: TagPageProps): Promise<Metadata> {
  const { slug } = await params;
  const tag = await resolveTag(slug);
  if (!tag) return { title: "Tag not found", robots: { index: false, follow: false } };
  return {
    title: `#${tag}`,
    description: `CodeLocal Blog articles tagged ${tag}.`,
    alternates: { canonical: `/blogs/tag/${slug}` },
  };
}

export default async function TagPage({ params }: TagPageProps) {
  const { slug } = await params;
  const [tag, posts] = await Promise.all([resolveTag(slug), getPostsByTagSlugForRender(slug)]);
  if (!tag) notFound();

  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>Tag</span>
        <h1>#{tag}</h1>
        <p>Every public CodeLocal Blog article connected to {tag}.</p>
      </header>
      <section className={styles.section} aria-labelledby="tagged-articles">
        <div className={styles.sectionHead}>
          <div><span className={styles.sectionLabel}>Archive</span><h2 id="tagged-articles">Tagged articles</h2></div>
          <p>{posts.length} {posts.length === 1 ? "article" : "articles"}</p>
        </div>
        <div className={styles.postGrid}>{posts.map((post) => <PostCard key={post.slug} post={post} compact />)}</div>
      </section>
    </main>
  );
}
