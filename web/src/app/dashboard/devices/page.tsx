import { LiveDevices } from "./devices-live";
import styles from "../dashboard.module.css";

export default function DevicesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Trusted machines</span>
          <h1>Devices</h1>
          <p>Review paired CodeLocal runtimes, search device history and revoke credentials you no longer trust.</p>
        </div>
        <span className={styles.productionLink}>Credential secrets stay server-side</span>
      </header>

      <LiveDevices />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Security boundary</span>
        <h2>Revocation is still enforced entirely by Go.</h2>
        <p>The browser sends only the public device ID plus the current CSRF token. Go resolves the private credential, requires a fresh security context, disconnects it and records the audit event.</p>
      </section>
    </section>
  );
}
