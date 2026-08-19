import { Suspense } from "react";
import { LiveCodeGraph } from "../code-graph-preview/code-graph-live";
import styles from "../dashboard.module.css";

export default function CodeGraphPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Local-runtime evidence</span>
          <h1>Code Graph</h1>
          <p>Bounded, revision-aware architecture and impact evidence returned by the selected authorized local checkout.</p>
        </div>
        <span className={styles.productionLink}>Source graph stays local</span>
      </header>

      <Suspense fallback={<section className={styles.livePanel}><span className={styles.eyebrow}>Code Graph</span><h2>Loading checkout context…</h2></section>}>
        <LiveCodeGraph />
      </Suspense>

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Privacy boundary</span>
        <h2>The browser receives a bounded projection, not the source graph.</h2>
        <p>
          Project roots, routing keys, repository IDs and source hashes stay outside the browser contract. Go resolves workspace authorization before querying the local runtime.
        </p>
      </section>
    </section>
  );
}
