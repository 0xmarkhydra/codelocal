import { LiveSecurity } from "./security-live";
import styles from "../dashboard.module.css";

export default function SecurityPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Security</h1></div></header>

      <LiveSecurity />
    </section>
  );
}
