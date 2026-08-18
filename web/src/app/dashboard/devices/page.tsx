import { LiveDevices } from "./devices-live";
import styles from "../dashboard.module.css";

export default function DevicesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Read-only route parity</span>
          <h1>Devices</h1>
          <p>Trusted machine runtimes from the Go device authority, without exposing credential material.</p>
        </div>
        <span className={styles.productionLink}>Revocation remains on Go</span>
      </header>

      <LiveDevices />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Mutation boundary</span>
        <h2>Device revocation stays on the existing Go flow for now.</h2>
        <p>The mutation requires the current CSRF and authorization path. Next.js only reads the minimal versioned DTO until that behavior has parity tests.</p>
      </section>
    </section>
  );
}
