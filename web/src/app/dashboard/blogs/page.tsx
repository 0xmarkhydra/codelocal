import Link from "next/link";
import { blogPosts, blogSeries, formatBlogDate } from "@/lib/blog";
import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import styles from "./blogs.module.css";

export default function BlogsPage() {
  const recentPosts = blogPosts.slice(0, 6);

  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.hero}>
          <span className={styles.heroIcon} aria-hidden="true">
            <AppIcon name="file" size={20} />
          </span>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>Publishing</span>
            <h1>Blogs</h1>
            <p>Your writing workspace inside CodeLocal. Public posts keep the same canonical Blog URLs.</p>
          </div>
          <Link className={styles.primaryAction} href="/blog">Open public blog</Link>
        </header>

        <div className={styles.stats} aria-label="Blog summary">
          <article>
            <strong>{blogPosts.length}</strong>
            <span>Published posts</span>
          </article>
          <article>
            <strong>{blogSeries.length}</strong>
            <span>Series</span>
          </article>
        </div>

        <section className={styles.section}>
          <div className={styles.sectionHeader}>
            <div>
              <span className={styles.kicker}>Content</span>
              <h2>Recent posts</h2>
            </div>
            <Link href="/blog">View all</Link>
          </div>

          <div className={styles.postList}>
            {recentPosts.map((post) => (
              <Link className={styles.postRow} href={`/blog/${post.slug}`} key={post.slug}>
                <div>
                  <span>{post.category}</span>
                  <strong>{post.title}</strong>
                  <p>{post.excerpt}</p>
                </div>
                <time dateTime={post.publishedAt}>{formatBlogDate(post.publishedAt)}</time>
              </Link>
            ))}
          </div>
        </section>
      </div>
    </section>
  );
}
