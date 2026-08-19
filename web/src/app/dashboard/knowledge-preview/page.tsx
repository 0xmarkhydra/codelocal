import Link from "next/link";
import { LiveKnowledgeHealth } from "./knowledge-health-live";
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
      <LiveKnowledgeHealth />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Cutover boundary</span>
        <h2>Knowledge feature parity is implemented here; production cutover is still gated.</h2>
        <p>
          Graph reads, aggregate Project Brain health and the existing CSRF-protected collective-learning mutation are now represented in Next.js.
          The main route remains on Go until real-edge cookie, User-Agent and trusted client-IP forwarding parity is verified with a rollback path.
        </p>
        <Link className={styles.liveAction} href="/dashboard/knowledge">Open current Go implementation</Link>
      </section>
    </section>
  );
}
