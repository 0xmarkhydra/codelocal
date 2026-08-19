import { LiveWorkspaces } from "./workspaces-live";
import styles from "../dashboard.module.css";

export default function WorkspacesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Local authorization</span>
          <h1>Workspaces</h1>
          <p>Search authorized project folders, open their local Code Graph and remove access without touching project files.</p>
        </div>
        <span className={styles.productionLink}>Local paths stay private</span>
      </header>

      <LiveWorkspaces />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Authorization boundary</span>
        <h2>Removing access keeps the existing local revocation handshake.</h2>
        <p>The action is available only while that machine runtime is reachable, requires CSRF plus a fresh Go security context, and never deletes or modifies files in the project folder.</p>
      </section>
    </section>
  );
}
