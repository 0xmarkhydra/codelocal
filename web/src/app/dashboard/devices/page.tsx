import { LiveDevices } from "./devices-live";
import styles from "../dashboard.module.css";

export default function DevicesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Machines</span>
          <h1>Devices</h1>
          <p>Máy đã pair, trạng thái kết nối và credential của CodeLocal.</p>
        </div>
      </header>
      <LiveDevices />
    </section>
  );
}
