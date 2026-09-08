import { LiveUsage } from "./usage-live";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function UsagePage() {
  const t = await getTranslations();
  return (
    <section className={styles.content}>
      <h1 className="sr-only">{t("Usage")}</h1>
      <LiveUsage />
    </section>
  );
}
