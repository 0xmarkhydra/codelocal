import dashboard from "../dashboard.module.css";
import { LeaderboardLive } from "./leaderboard-live";
import { getTranslations } from "@/lib/i18n/server";

export default async function LeaderboardPage() {
  const t = await getTranslations();
  return <section className={dashboard.content}>
    <h1 className="sr-only">{t("Leaderboard")}</h1>
    <LeaderboardLive />
  </section>;
}
