import Link from "next/link";
import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function CodeGraphPage() {
  const t = await getTranslations();
  return (
    <section className={styles.content}>
      <div className={styles.brainTopbar}>
        <nav className={styles.segmentedNav} aria-label={t("Brain views")}>
          <Link href="/dashboard/knowledge">{t("Knowledge")}</Link>
          <span aria-current="page">{t("Code")}</span>
        </nav>
      </div>
      <Suspense fallback={<DashboardResourceFeedback kind="loading" label="Code Brain" />}>
        <LiveCodeGraph />
      </Suspense>
    </section>
  );
}
