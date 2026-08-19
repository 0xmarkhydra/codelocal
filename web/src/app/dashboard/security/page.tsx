import { LiveSecurity } from "./security-live";
import styles from "../dashboard.module.css";

export default function SecurityPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Security control center</span>
          <h1>Security</h1>
          <p>Session risk, device trust and workspace authorization without exposing private audit telemetry.</p>
        </div>
        <span className={styles.productionLink}>Security authority · Go</span>
      </header>

      <LiveSecurity />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Authority boundary</span>
        <h2>This page is presentation, not a second security system.</h2>
        <p>
          Session evaluation, CSRF validation, password verification, rate limiting, credential revocation and workspace authorization remain server-side Go responsibilities.
        </p>
      </section>
    </section>
  );
}
