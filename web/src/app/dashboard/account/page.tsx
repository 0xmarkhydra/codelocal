import { Suspense } from "react";
import { LiveAccount } from "./account-live";
import styles from "../dashboard.module.css";

export default function AccountPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Account</span>
          <h1>Account</h1>
          <p>Manage your identity, password and account security.</p>
        </div>
        <span className={styles.productionLink}>Protected account</span>
      </header>

      <Suspense fallback={<section className={styles.livePanel}><span className={styles.eyebrow}>Account</span><h2>Loading account state…</h2></section>}>
        <LiveAccount />
      </Suspense>

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Protected changes</span>
        <h2>Password changes stay inside the secure account flow.</h2>
        <p>
          Current-password checks, rate limits, session rotation and audit happen behind the interface. The page never stores your password.
        </p>
      </section>
    </section>
  );
}
