import { getTranslations } from "@/lib/i18n/server";
import dashboard from "../dashboard.module.css";
import { ShotsHub } from "./shots-hub";
import styles from "./shots.module.css";

export default async function ShotsPage() {
  const t = await getTranslations();
  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <h1 className="sr-only">{t("Shots")}</h1>
        <ShotsHub />
      </div>
    </section>
  );
}
