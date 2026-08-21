import { LiveOverview } from "./overview-live";
import styles from "./dashboard.module.css";

export default function DashboardPage() {
  return (
    <section className={styles.content}>
      <header className={`${styles.header} ${styles.compactHeader}`}>
        <div>
          <span className={styles.eyebrow}>Workspace</span>
          <h1>Home</h1>
          <p>Project brain, runtime and MCP activity in one view.</p>
        </div>
      </header>

      <LiveOverview />
    </section>
  );
}