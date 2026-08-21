import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import styles from "../dashboard.module.css";

export default function CodeGraphPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Code Graph</h1></div></header>
      <Suspense fallback={<section className={styles.livePanel}><h2>Loading…</h2></section>}>
        <LiveCodeGraph />
      </Suspense>
    </section>
  );
}
