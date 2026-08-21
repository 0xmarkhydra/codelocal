import { Suspense } from "react";
import { LiveAccount } from "./account-live";
import styles from "../dashboard.module.css";

export default function AccountPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Account</h1></div></header>
      <Suspense fallback={<section className={styles.livePanel}><h2>Loading…</h2></section>}>
        <LiveAccount />
      </Suspense>
    </section>
  );
}
