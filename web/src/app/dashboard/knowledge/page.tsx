import { LiveKnowledgeHealth } from "./knowledge-health-live";
import { LiveKnowledgeGraph } from "./knowledge-live";
import styles from "../dashboard.module.css";

export default function KnowledgePage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Project Brain</span>
          <h1>Knowledge Graph</h1>
          <p>Durable project knowledge, health and collective controls rendered by Next.js with Go as the data and security authority.</p>
        </div>
        <span className={styles.productionLink}>Bounded graph · Go authority</span>
      </header>

      <LiveKnowledgeGraph />
      <LiveKnowledgeHealth />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Authority boundary</span>
        <h2>Canonical knowledge and collective mutations remain server-side.</h2>
        <p>
          The browser receives privacy-minimized graph data and aggregate health only. Collective preference changes continue to use the existing CSRF-protected Go mutation path.
        </p>
      </section>
    </section>
  );
}
