import { LiveKnowledgeHealth } from "./knowledge-health-live";
import { LiveKnowledgeGraph } from "./knowledge-live";
import styles from "../dashboard.module.css";

export default function KnowledgePage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Knowledge</h1></div></header>
      <LiveKnowledgeGraph />
      <LiveKnowledgeHealth />
    </section>
  );
}
