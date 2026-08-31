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
            <AppIcon name="blog" size={20} />
          </span>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>Publishing workspace</span>
            <h1>Blogs</h1>
            <p>Create, organize and publish CodeLocal articles without leaving the dashboard.</p>
          </div>
          <Link className={styles.primaryAction} href="/blogs">
            <span>View public blog</span>
            <AppIcon name="chevron-right" size={14} />
          </Link>
        </header>
        <BlogsHub />
      </div>
    </section>
  );
}
