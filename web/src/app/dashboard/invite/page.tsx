import dashboard from "../dashboard.module.css";
import { InviteLive } from "./invite-live";
import { getTranslations } from "@/lib/i18n/server";

export default async function InvitePage() {
  const t = await getTranslations();
  return (
    <section className={dashboard.content}>
      <h1 className="sr-only">{t("Invite")}</h1>
      <InviteLive />
    </section>
  );
}
