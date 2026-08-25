"use client";

import { AppIcon } from "./app-icon";
import styles from "./dashboard.module.css";

type FeedbackProps =
  | { kind: "loading"; label: string }
  | { kind: "unauthenticated"; label: string }
  | { kind: "error"; label: string; message: string; onRetry: () => void };

export function DashboardResourceFeedback(props: FeedbackProps) {
  if (props.kind === "loading") {
    return (
      <section className={styles.resourceFeedback} aria-live="polite" aria-label={`Đang tải ${props.label}`}>
        <span className={styles.resourceFeedbackSpinner} aria-hidden="true"><i /><i /><i /></span>
        <strong>Đang tải {props.label}</strong>
        <span>Dữ liệu sẽ xuất hiện ngay khi CodeLocal phản hồi.</span>
      </section>
    );
  }

  if (props.kind === "unauthenticated") {
    return (
      <section className={styles.resourceFeedback} aria-live="polite">
        <span className={styles.resourceFeedbackIcon} aria-hidden="true"><AppIcon name="shield" /></span>
        <strong>Cần đăng nhập</strong>
        <span>Đăng nhập để tiếp tục với {props.label}.</span>
        <a className={styles.resourceFeedbackAction} href="/login?next=%2Fdashboard">Tiếp tục</a>
      </section>
    );
  }

  return (
    <section className={styles.resourceFeedback} aria-live="polite">
      <span className={styles.resourceFeedbackIcon} aria-hidden="true"><AppIcon name="refresh" /></span>
      <strong>Chưa kết nối được</strong>
      <span>{props.label} chưa phản hồi. Bạn có thể thử lại.</span>
      <button className={styles.resourceFeedbackAction} type="button" onClick={props.onRetry}>Thử lại</button>
    </section>
  );
}