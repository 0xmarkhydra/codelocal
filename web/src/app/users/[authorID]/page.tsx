import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { PostCard } from "../../blog/_components";
import styles from "../../blog/blog.module.css";
import { getBlogPostsByAuthorForRender } from "@/lib/blog-server";

type AuthorPageProps = { params: Promise<{ authorID: string }> };

export async function generateMetadata({ params }: AuthorPageProps): Promise<Metadata> {
  const { authorID } = await params;
  const posts = await getBlogPostsByAuthorForRender(authorID);
  const author = posts[0]?.author;
  if (!author) return { title: "Author not found", robots: { index: false, follow: false } };
  return {
    title: `${author.name} · CodeLocal Blog`,
    description: `Public CodeLocal Blog posts by ${author.name}.`,
    alternates: { canonical: `/users/${encodeURIComponent(authorID)}` },
  };
}

export default async function AuthorPage({ params }: AuthorPageProps) {
  const { authorID } = await params;
  const posts = await getBlogPostsByAuthorForRender(authorID);
  if (posts.length === 0) notFound();
  const author = posts[0].author;

  return (
    <main className={styles.page}>
      <header className={styles.taxonomyHeader}>
        <span className={styles.eyebrow}>Author</span>
        <h1>{author.name}</h1>
        <p>{author.role}. Public articles and learning notes published on CodeLocal.</p>
      </header>
      <section className={styles.section} aria-labelledby="author-posts">
        <div className={styles.sectionHead}>
          <div><span className={styles.sectionLabel}>Posts</span><h2 id="author-posts">Published articles</h2></div>
          <p>{posts.length} {posts.length === 1 ? "article" : "articles"}</p>
        </div>
        <div className={styles.postGrid}>{posts.map((post) => <PostCard key={post.slug} post={post} compact />)}</div>
      </section>
    </main>
  );
}
