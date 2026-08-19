import { LiveOverview } from "./overview-live";
import styles from "./dashboard.module.css";

export default function DashboardPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Workspace</span>
          <h1>Overview</h1>
          <p>Your machines, authorized projects and recent MCP activity.</p>
        </div>
      </header>

      <LiveOverview />
    </section>
  );
}