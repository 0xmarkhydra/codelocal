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
        <span className={styles.productionLink}>Protected by CodeLocal</span>
      </header>

      <LiveSecurity />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Security boundary</span>
        <h2>Security rules are enforced beyond this screen.</h2>
        <p>
          Session checks, password protection, device revocation and workspace permissions are enforced by CodeLocal’s security layer.
        </p>
      </section>
    </section>
  );
}
