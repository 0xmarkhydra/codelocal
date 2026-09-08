import { LiveDevices } from "./devices-live";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function DevicesPage() {
  const t = await getTranslations();
  return (
    <section className={styles.content}>
      <h1 className="sr-only">{t("Devices")}</h1>
      <LiveDevices />
    </section>
  );
}
