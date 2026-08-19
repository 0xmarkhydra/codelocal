import Link from "next/link";
import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import styles from "../dashboard.module.css";

export default function CodeGraphPreviewPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Preview route · local-runtime parity</span>
          <h1>Code Graph Preview</h1>
          <p>Next.js control surface over bounded, revision-aware evidence returned by the selected authorized local checkout.</p>
        </div>
        <Link className={styles.productionLink} href="/dashboard/code-graph">Canonical Code Graph</Link>
      </header>

      <Suspense fallback={<section className={styles.livePanel}><span className={styles.eyebrow}>Code Graph</span><h2>Loading checkout context…</h2></section>}>
        <LiveCodeGraph />
      </Suspense>

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Privacy boundary</span>
        <h2>The full graph and raw source remain local.</h2>
        <p>
          Browser responses use workspace-relative paths and response-local node/edge IDs. Project roots, routing keys, repository IDs,
          source hashes and the full local graph are not exposed by the web contract.
        </p>
      </section>
    </section>
  );
}
