import { LiveUsage } from "./usage-live";
import styles from "../dashboard.module.css";

export default function UsagePage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Activity</span>
          <h1>Usage</h1>
          <p>Estimated MCP tool activity and token volume, separate from provider billing.</p>
        </div>
        <span className={styles.productionLink}>Usage telemetry</span>
      </header>

      <LiveUsage />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Usage boundary</span>
        <h2>Usage is activity data, not an invoice.</h2>
        <p>These estimates summarize MCP tool calls and payload. They do not calculate provider charges or change billing access.</p>
      </section>
    </section>
  );
}
