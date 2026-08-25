import { LiveOverview } from "./overview-live";
import { DashboardChat } from "./dashboard-chat";
import styles from "./dashboard.module.css";

export default function DashboardPage() {
  return (
    <section className={styles.content}>
      <header className={`${styles.header} ${styles.compactHeader}`}>
        <div>
          <h1>Chat</h1>
          <p>CodeLocal · Project Brain · local tools</p>
        </div>
      </header>

      <DashboardChat />
      <div style={{ marginTop: 14 }}>
        <LiveOverview />
      </div>
    </section>
  );
}
