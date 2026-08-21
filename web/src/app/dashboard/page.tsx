import { LiveOverview } from "./overview-live";
import styles from "./dashboard.module.css";

export default function DashboardPage() {
  return (
    <section className={styles.content}>
      <header className={`${styles.header} ${styles.compactHeader}`}>
        <div><h1>Home</h1></div>
      </header>

      <LiveOverview />
    </section>
  );
}