"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";
import { useTranslations } from "@/lib/i18n/provider";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";

type Entry = { rank: number; emailMasked: string; initial: string; calls: number; tokens: number; isCurrent: boolean };
type Leaderboard = { windowDays: number; currentRank: number; currentTokens: number; entries: Entry[] };

function isLeaderboard(value: unknown): value is Leaderboard {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return typeof v.windowDays === "number" && typeof v.currentRank === "number" && typeof v.currentTokens === "number" && Array.isArray(v.entries);
}

export function LeaderboardLive() {
  const { locale, t } = useTranslations();
  const number = new Intl.NumberFormat(locale);
  const compact = (value: number) => new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: 1 }).format(value);
  const router = useRouter();
  const [data, setData] = useState<Leaderboard | null>(null);
  const [error, setError] = useState(false);
  useEffect(() => {
    fetch("/api/v1/leaderboard", { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (response.status === 401) { router.replace(`/login?next=${encodeURIComponent("/dashboard/leaderboard")}`); throw new Error("unauthorized"); }
        if (!response.ok) throw new Error("unavailable");
        return response.json() as Promise<unknown>;
      })
      .then((payload) => { if (!isLeaderboard(payload)) throw new Error("invalid"); setData(payload); })
      .catch((reason) => { if (reason instanceof Error && reason.message !== "unauthorized") setError(true); });
  }, [router]);
  if (error) return <DashboardResourceFeedback kind="error" label={t("Leaderboard")} message={t("Unavailable")} onRetry={() => window.location.reload()} />;
  if (!data) return <DashboardResourceFeedback kind="loading" label={t("Leaderboard")} />;
  return <div className={surface.grid}>
    <div className={surface.metrics}>
      <div className={surface.metric}><span>{t("Your rank")}</span><strong>{data.currentRank ? `#${number.format(data.currentRank)}` : "—"}</strong></div>
      <div className={surface.metric}><span>{t("Your estimated payload")}</span><strong>{compact(data.currentTokens)}</strong></div>
      <div className={surface.metric}><span>{t("Ranked users")}</span><strong>{number.format(data.entries.length)}</strong></div>
    </div>
    <section className={surface.card}>
      <span className={dashboard.eyebrow}>{t("Top users · {count} days", { count: data.windowDays })}</span>
      <h2 className={surface.title}>{t("Estimated MCP payload")}</h2>
      <p className={surface.copy}>{t("Emails are masked. This ranking reflects CodeLocal.Cloud MCP payload estimates, not model-provider billing.")}</p>
      {data.entries.length === 0 ? <div className={surface.empty}>{t("No MCP usage recorded in this period.")}</div> : <div className={surface.list}>{data.entries.map((entry) => <div className={surface.rankRow} key={`${entry.rank}-${entry.emailMasked}`}><span className={surface.rank}>#{number.format(entry.rank)}</span><div className={surface.identity}><strong>{entry.isCurrent ? `${t("You")} · ` : ""}{entry.initial} · {entry.emailMasked}</strong><small>{t("{count} estimated tokens", { count: entry.tokens })}</small></div>{entry.isCurrent && <span className={`${surface.badge} ${surface.badgeBlue}`}>{t("You")}</span>}<span className={`${surface.value} ${surface.calls}`}>{t("{count} calls", { count: entry.calls })}</span></div>)}</div>}
    </section>
  </div>;
}
