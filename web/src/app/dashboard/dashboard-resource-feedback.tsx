"use client";

import styles from "./dashboard.module.css";

type FeedbackProps =
  | { kind: "loading"; label: string }
  | { kind: "unauthenticated"; label: string }
  | { kind: "error"; label: string; message: string; onRetry: () => void };

export function DashboardResourceFeedback(props: FeedbackProps) {
  if (props.kind === "loading") {
    return (
      <section className={styles.livePanel} aria-live="polite">
        <span className={styles.eyebrow}>{props.label}</span>
        <h2>Checking CodeLocal…</h2>
        <p>No local placeholder values are shown while real state is loading.</p>
      </section>
    );
  }

  if (props.kind === "unauthenticated") {
    return (
      <section className={styles.livePanel} aria-live="polite">
        <span className={styles.eyebrow}>{props.label}</span>
        <h2>Sign in to load real CodeLocal state.</h2>
        <p>Your signed-in session is being verified before private data is shown.</p>
        <a className={styles.liveAction} href="/login?next=%2Fdashboard">Sign in through CodeLocal</a>
      </section>
    );
  }

  return (
    <section className={styles.livePanel} aria-live="polite">
      <span className={styles.eyebrow}>{props.label}</span>
      <h2>Live backend state is unavailable.</h2>
      <p>{props.message} The UI will not substitute mocked activity.</p>
      <button className={styles.liveAction} type="button" onClick={props.onRetry}>Retry</button>
    </section>
  );
}
