import { Suspense } from "react";
import { LiveAccount } from "./account-live";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";

export default function AccountPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Account</h1></div></header>
      <Suspense fallback={<DashboardResourceFeedback kind="loading" label="Account" />}>
        <LiveAccount />
      </Suspense>
    </section>
  );
}