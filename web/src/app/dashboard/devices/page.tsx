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
        <span className={styles.productionLink}>Paired runtimes</span>
      </header>

      <LiveDevices />

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Revocation</span>
        <h2>Revoking a device disconnects it from CodeLocal.</h2>
        <p>The device credential is invalidated and the runtime is disconnected. Your local project files are not modified.</p>
      </section>
    </section>
  );
}
