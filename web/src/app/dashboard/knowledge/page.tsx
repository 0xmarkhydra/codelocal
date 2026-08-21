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
          <p>Durable project context your AI can reuse across sessions, tools and teammates.</p>
        </div>
        <span className={styles.productionLink}>Private by design</span>
      </header>

      <LiveKnowledgeGraph />
      <LiveKnowledgeHealth />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Privacy boundary</span>
        <h2>Your raw project stays out of the browser.</h2>
        <p>
          The graph receives only minimized project knowledge and aggregate health. Preference changes are protected and applied through CodeLocal’s secure account flow.
        </p>
      </section>
    </section>
  );
}
