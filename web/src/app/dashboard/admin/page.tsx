import dashboard from "../dashboard.module.css";
import { AdminLive } from "./admin-live";
import { getTranslations } from "@/lib/i18n/server";

export default async function AdminPage() {
  const t = await getTranslations();
  return (
    <section className={dashboard.content}>
      <h1 className="sr-only">{t("Administration")}</h1>
      <AdminLive />
    </section>
  );
}
