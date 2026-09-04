import { Suspense } from "react";
import { DashboardChat } from "./dashboard-chat";
import styles from "./dashboard.module.css";

export default function DashboardPage() {
  return (
    <section className={`${styles.content} ${styles.chatContent}`}>
      <Suspense fallback={<div className={styles.chatLoading}>Đang mở CodeLocal…</div>}>
        <DashboardChat />
      </Suspense>
    </section>
  );
}
