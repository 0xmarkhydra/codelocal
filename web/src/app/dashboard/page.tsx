import { LiveOverview } from "./overview-live";
import styles from "./dashboard.module.css";

export default function DashboardPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Private control plane</span>
          <h1>Overview</h1>
          <p>Your paired machines, authorized project folders and MCP activity in one place.</p>
        </div>
        <span className={styles.productionLink}>Go authority · Next presentation</span>
      </header>

      <LiveOverview />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Authority boundary</span>
        <h2>The browser presents state; it does not become the source of truth.</h2>
        <p>
          Identity, authorization, workspace access, device credentials, MCP usage and sensitive mutations remain enforced by the Go backend and local runtime boundaries.
        </p>
      </section>
    </section>
  );
}
