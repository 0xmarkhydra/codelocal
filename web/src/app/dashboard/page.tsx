import Link from "next/link";
import { AppIcon } from "./app-icon";
import { LiveOverview } from "./overview-live";
import styles from "./overview.module.css";
import { getTranslations } from "@/lib/i18n/server";

export async function generateMetadata() {
  const t = await getTranslations();
  return { title: t("Overview") };
}

export default async function DashboardPage() {
  const t = await getTranslations();
  return (
    <section className={styles.page}>
      <h1 className="sr-only">{t("Overview")}</h1>
      <div className={styles.content}>
        <LiveOverview />
        <Link className={styles.brainLink} href="/dashboard/knowledge">
          <AppIcon name="brain" size={24} />
          <div><strong>Project Brain</strong><span>{t("Knowledge, context and experience for your projects.")}</span></div>
          <AppIcon name="chevron-right" size={18} />
        </Link>
        <footer className={styles.footer}>
          <span>
            <AppIcon name="shield" size={13} /> {t("Local execution. Workspace-scoped access.")}
          </span>
          <Link href="/support">
            {t("Need help?")} <span aria-hidden="true">↗</span>
          </Link>
        </footer>
      </div>
    </section>
  );
}
