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
          <p>Identity and password controls rendered by Next.js while security authority remains in Go.</p>
        </div>
        <span className={styles.productionLink}>Password security · Go</span>
      </header>

      <Suspense fallback={<section className={styles.livePanel}><span className={styles.eyebrow}>Account</span><h2>Loading account state…</h2></section>}>
        <LiveAccount />
      </Suspense>

      <section className={styles.routeGuardrail}>
        <span className={styles.eyebrow}>Security boundary</span>
        <h2>Next.js never verifies or stores the password.</h2>
        <p>
          Current-password verification, CSRF, fresh-security checks, rate limiting, password hashing, security-version rotation,
          session replacement and audit remain inside the existing Go authentication handler.
        </p>
      </section>
    </section>
  );
}
