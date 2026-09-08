import Link from "next/link";
import { LiveKnowledgeGraph } from "./knowledge-live";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function KnowledgePage() {
  const t = await getTranslations();
  return (
    <section className={`${styles.content} ${styles.brainContent}`}>
      <div className={styles.brainTopbar}>
        <nav className={styles.segmentedNav} aria-label={t("Brain views")}>
          <span aria-current="page">{t("Knowledge")}</span>
          <Link href="/dashboard/code-graph">{t("Code")}</Link>
        </nav>
      </div>
      <div className={styles.brainStage}>
        <LiveKnowledgeGraph />
      </div>
    </section>
  );
}
