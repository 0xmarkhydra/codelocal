import Link from "next/link";
import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";

export default function CodeGraphPage() {
  return (
    <section className={`${styles.content} ${styles.brainContent}`}>
      <header className={`${styles.header} ${styles.brainHeader}`}>
        <div>
          <span className={styles.eyebrow}>Intelligence</span>
          <h1>Brain</h1>
          <p>Xem cấu trúc code và quan hệ trong workspace đã chọn.</p>
        </div>
        <nav className={styles.segmentedNav} aria-label="Brain views">
          <Link href="/dashboard/knowledge">Knowledge</Link>
          <span aria-current="page">Code</span>
        </nav>
      </header>
      <Suspense fallback={<DashboardResourceFeedback kind="loading" label="Code Brain" />}>
        <LiveCodeGraph />
      </Suspense>
    </section>
  );
}