import Link from "next/link";
import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import { BlogsHub } from "./blogs-hub";
import styles from "./blogs.module.css";

export default function BlogsPage() {
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
            <p>Write, publish and manage posts from one CodeLocal Blog workspace.</p>
          </div>
          <Link className={styles.primaryAction} href="/blog">Open public blog</Link>
        </header>
        <BlogsHub />
      </div>
    </section>
  );
}
