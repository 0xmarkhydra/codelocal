import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import styles from "../dashboard.module.css";

export default function CodeGraphPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Local-runtime evidence</span>
          <h1>Code Graph</h1>
          <p>Explore symbols, relationships and impact inside an authorized local workspace.</p>
        </div>
        <span className={styles.productionLink}>Computed locally</span>
      </header>

      <Suspense fallback={<section className={styles.livePanel}><span className={styles.eyebrow}>Code Graph</span><h2>Loading checkout context…</h2></section>}>
        <LiveCodeGraph />
      </Suspense>

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Privacy boundary</span>
        <h2>The source graph stays on your machine.</h2>
        <p>
          CodeLocal sends only the bounded relationships needed for this view. Project roots, routing keys, repository IDs and source hashes remain local.
        </p>
      </section>
    </section>
  );
}
