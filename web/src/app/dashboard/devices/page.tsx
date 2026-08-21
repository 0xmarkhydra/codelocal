import { LiveDevices } from "./devices-live";
import styles from "../dashboard.module.css";

export default function DevicesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Devices</h1></div></header>
      <LiveDevices />
    </section>
  );
}
