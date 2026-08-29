import Link from "next/link";
import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";

export default function CodeGraphPage() {
  return (
    <section className={`${styles.content} ${styles.brainContent}`}>
      <div className={styles.brainTopbar}>
        <nav className={styles.segmentedNav} aria-label="Brain views">
          <Link href="/dashboard/knowledge">Knowledge</Link>
          <span aria-current="page">Code</span>
        </nav>
      </div>
      <Suspense fallback={<DashboardResourceFeedback kind="loading" label="Code Brain" />}>
        <LiveCodeGraph />
      </Suspense>
    </section>
  );
}
