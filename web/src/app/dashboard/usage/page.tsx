import { LiveUsage } from "./usage-live";
import styles from "../dashboard.module.css";

export default function UsagePage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Usage</h1></div></header>
      <LiveUsage />
    </section>
  );
}
