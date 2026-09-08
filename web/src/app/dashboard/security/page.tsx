import { LiveSecurity } from "./security-live";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function SecurityPage() {
  const t = await getTranslations();
  return (
    <section className={styles.content}>
      <h1 className="sr-only">{t("Security")}</h1>

      <LiveSecurity />
    </section>
  );
}
