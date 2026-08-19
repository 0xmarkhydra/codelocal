import Link from "next/link";
import { LiveKnowledgePreview } from "./knowledge-preview-live";
import styles from "../dashboard.module.css";

export default function KnowledgePreviewPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Preview route · read-only parity</span>
          <h1>Knowledge Graph Preview</h1>
          <p>Next.js visualization over a privacy-minimized, bounded graph DTO from the Go Project Brain authority.</p>
        </div>
        <Link className={styles.productionLink} href="/dashboard/knowledge">Current Project Brain controls</Link>
      </header>

      <LiveKnowledgePreview />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Cutover boundary</span>
        <h2>The main Knowledge Graph route still belongs to Go.</h2>
        <p>
          Project Brain health, canonical-index diagnostics, semantic rollout state and collective-learning controls have not reached Next.js parity yet.
          This preview intentionally does not replace or hide those existing controls.
        </p>
        <Link className={styles.liveAction} href="/dashboard/knowledge">Open current health &amp; controls</Link>
      </section>
    </section>
  );
}
