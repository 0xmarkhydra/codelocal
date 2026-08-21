"use client";

import styles from "./dashboard.module.css";

type FeedbackProps =
  | { kind: "loading"; label: string }
  | { kind: "unauthenticated"; label: string }
  | { kind: "error"; label: string; message: string; onRetry: () => void };

export function DashboardResourceFeedback(props: FeedbackProps) {
  if (props.kind === "loading") {
    return <section className={styles.livePanel} aria-live="polite"><h2>Loading…</h2></section>;
  }

  if (props.kind === "unauthenticated") {
    return (
      <section className={styles.livePanel} aria-live="polite">
        <h2>Sign in</h2>
        <a className={styles.liveAction} href="/login?next=%2Fdashboard">Continue</a>
      </section>
    );
  }

  return (
    <section className={styles.livePanel} aria-live="polite">
      <h2>Unavailable</h2>
      <button className={styles.liveAction} type="button" onClick={props.onRetry}>Retry</button>
    </section>
  );
}
