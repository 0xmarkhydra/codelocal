"use client";

import { AppIcon } from "./app-icon";
import styles from "./dashboard.module.css";
import { useTranslations } from "@/lib/i18n/provider";

type FeedbackProps =
  | { kind: "loading"; label: string }
  | { kind: "unauthenticated"; label: string }
  | { kind: "error"; label: string; message: string; onRetry: () => void };

export function DashboardResourceFeedback(props: FeedbackProps) {
  const { t } = useTranslations();
  if (props.kind === "loading") {
    return (
      <section className={styles.resourceFeedback} aria-live="polite" aria-label={t("Loading")}>
        <span className={styles.resourceFeedbackSpinner} aria-hidden="true"><i /><i /><i /></span>
        <strong>{t("Loading")}</strong>
        <span>{t("Data will appear when CodeLocal.Cloud responds.")}</span>
      </section>
    );
  }

  if (props.kind === "unauthenticated") {
    return (
      <section className={styles.resourceFeedback} aria-live="polite">
        <span className={styles.resourceFeedbackIcon} aria-hidden="true"><AppIcon name="shield" /></span>
        <strong>{t("Sign in required")}</strong>
        <span>{t("Sign in to continue.")}</span>
        <a className={styles.resourceFeedbackAction} href="/login?next=%2Fdashboard">{t("Continue")}</a>
      </section>
    );
  }

  return (
    <section className={styles.resourceFeedback} aria-live="polite">
      <span className={styles.resourceFeedbackIcon} aria-hidden="true"><AppIcon name="refresh" /></span>
      <strong>{t("Unable to connect")}</strong>
      <span>{t("CodeLocal.Cloud has not responded. Try again.")}</span>
      <button className={styles.resourceFeedbackAction} type="button" onClick={props.onRetry}>{t("Retry")}</button>
    </section>
  );
}
