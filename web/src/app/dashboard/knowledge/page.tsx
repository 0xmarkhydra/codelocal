import Link from "next/link";
import { LiveKnowledgeGraph } from "./knowledge-live";
import styles from "../dashboard.module.css";

export default function KnowledgePage() {
  return (
    <section className={`${styles.content} ${styles.brainContent}`}>
      <div className={styles.brainTopbar}>
        <nav className={styles.segmentedNav} aria-label="Brain views">
          <span aria-current="page">Knowledge</span>
          <Link href="/dashboard/code-graph">Code</Link>
        </nav>
      </div>
      <div className={styles.brainStage}>
        <LiveKnowledgeGraph />
      </div>
    </section>
  );
}
