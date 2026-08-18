import { LiveUsage } from "./usage-live";
import styles from "../dashboard.module.css";

export default function UsagePage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Read-only route parity</span>
          <h1>Usage</h1>
          <p>Estimated MCP tool payload from the Go usage store, separated from provider billing.</p>
        </div>
        <span className={styles.productionLink}>No inferred USD cost</span>
      </header>

      <LiveUsage />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Billing boundary</span>
        <h2>Usage telemetry is not an entitlement or invoice source.</h2>
        <p>Billing access remains driven only by verified server-side billing state. This page reports estimated MCP payload and tool calls; it does not estimate provider charges or grant paid access.</p>
      </section>
    </section>
  );
}
