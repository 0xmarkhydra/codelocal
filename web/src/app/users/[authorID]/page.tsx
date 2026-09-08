import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { BlogFooter, BlogHeader, PostCard } from "../../blog/_components";
import styles from "../../blog/blog.module.css";
import { getBlogPostsByAuthorForRender } from "@/lib/blog-server";
import { getTranslations } from "@/lib/i18n/server";

type AuthorPageProps = { params: Promise<{ authorID: string }> };

export async function generateMetadata({ params }: AuthorPageProps): Promise<Metadata> {
  const t = await getTranslations();
  const { authorID } = await params;
  const posts = await getBlogPostsByAuthorForRender(authorID);
  const author = posts[0]?.author;
  if (!author) return { title: t("Author not found"), robots: { index: false, follow: false } };
  return {
    title: `${author.name} · ${t("CodeLocal Blog")}`,
    description: t("Public CodeLocal Blog posts by {author}.", { author: author.name }),
    alternates: { canonical: `/users/${encodeURIComponent(authorID)}` },
  };
}

export default async function AuthorPage({ params }: AuthorPageProps) {
  const t = await getTranslations();
  const { authorID } = await params;
  const posts = await getBlogPostsByAuthorForRender(authorID);
  if (posts.length === 0) notFound();
  const author = posts[0].author;

  return (
    <div className={styles.blogShell}>
      <BlogHeader />
      <main className={styles.page}>
        <header className={styles.taxonomyHeader}>
          <span className={styles.eyebrow}>{t("Author")}</span>
          <h1>{author.name}</h1>
          <p>{author.role}</p>
        </header>
        <section className={styles.section} aria-labelledby="author-posts">
          <div className={styles.sectionHead}>
            <h2 id="author-posts">{t("Published articles")}</h2>
            <p>{t("{count} articles", { count: posts.length })}</p>
          </div>
          <div className={styles.postGrid}>{posts.map((post) => <PostCard key={post.slug} post={post} compact />)}</div>
        </section>
      </main>
      <BlogFooter />
    </div>
  );
}
