import { LiveWorkspaces } from "./workspaces-live";
import styles from "../dashboard.module.css";

export default function WorkspacesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Read-only route parity</span>
          <h1>Workspaces</h1>
          <p>Authorized project folders from the Go workspace authority, without exposing local filesystem paths.</p>
        </div>
        <span className={styles.productionLink}>Mutations remain on Go</span>
      </header>

      <LiveWorkspaces />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Mutation boundary</span>
        <h2>Removing workspace access is intentionally not ported yet.</h2>
        <p>The current Go flow keeps CSRF and local-runtime checks authoritative. This Next route is read-only until mutation parity is explicitly verified.</p>
      </section>
    </section>
  );
}
