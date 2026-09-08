import { Suspense } from "react";
import { LiveAccount } from "./account-live";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function AccountPage() {
  const t = await getTranslations();
  return (
    <section className={styles.content}>
      <h1 className="sr-only">{t("Account")}</h1>
      <Suspense fallback={<DashboardResourceFeedback kind="loading" label={t("Account")} />}>
        <LiveAccount />
      </Suspense>
    </section>
  );
}
