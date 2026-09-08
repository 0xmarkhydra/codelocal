import Link from "next/link";
import { getTranslations } from "@/lib/i18n/server";
import dashboard from "../dashboard.module.css";
import { BlogsHub } from "./blogs-hub";
import styles from "./blogs.module.css";

export default async function BlogsPage() {
  const t = await getTranslations();
  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <h1 className="sr-only">{t("Blogs")}</h1>
        <Link className={styles.primaryAction} href="/blogs">{t("Open public blog")}</Link>
        <BlogsHub />
      </div>
    </section>
  );
}
