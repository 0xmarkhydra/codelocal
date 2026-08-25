import Link from "next/link";
import { AppIcon } from "../app-icon";
import { LiveKnowledgeHealth } from "./knowledge-health-live";
import { LiveKnowledgeGraph } from "./knowledge-live";
import styles from "../dashboard.module.css";

export default function KnowledgePage() {
  return (
    <section className={`${styles.content} ${styles.brainContent}`}>
      <header className={`${styles.header} ${styles.brainHeader}`}>
        <div>
          <span className={styles.eyebrow}>Intelligence</span>
          <h1>Brain</h1>
          <p>Khám phá tri thức, dự án và các mối liên hệ CodeLocal đã ghi nhớ.</p>
        </div>
        <nav className={styles.segmentedNav} aria-label="Brain views">
          <span aria-current="page">Knowledge</span>
          <Link href="/dashboard/code-graph">Code</Link>
        </nav>
      </header>

      <div className={styles.brainStage}>
        <LiveKnowledgeGraph />
      </div>

      <details className={styles.brainHealthDisclosure}>
        <summary>
          <span>Brain health & controls</span>
          <AppIcon name="chevron-down" size={16} />
        </summary>
        <LiveKnowledgeHealth />
      </details>
    </section>
  );
}
