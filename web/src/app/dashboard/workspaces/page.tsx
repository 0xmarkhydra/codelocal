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
        <span className={styles.productionLink}>Local access</span>
      </header>

      <LiveWorkspaces />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Access control</span>
        <h2>Removing access does not touch your files.</h2>
        <p>CodeLocal removes authorization for that workspace only. The project folder and its contents remain unchanged on your machine.</p>
      </section>
    </section>
  );
}
