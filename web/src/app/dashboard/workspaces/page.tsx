import { LiveWorkspaces } from "./workspaces-live";
import styles from "../dashboard.module.css";
import { getTranslations } from "@/lib/i18n/server";

export default async function WorkspacesPage() {
  const t = await getTranslations();
  return (
    <section className={styles.content}>
      <h1 className="sr-only">{t("Projects")}</h1>
      <LiveWorkspaces />
    </section>
  );
}
