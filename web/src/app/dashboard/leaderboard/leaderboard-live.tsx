"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";

type Entry = { rank: number; emailMasked: string; initial: string; calls: number; tokens: number; isCurrent: boolean };
type Leaderboard = { windowDays: number; currentRank: number; currentTokens: number; entries: Entry[] };

function isLeaderboard(value: unknown): value is Leaderboard {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return typeof v.windowDays === "number" && typeof v.currentRank === "number" && typeof v.currentTokens === "number" && Array.isArray(v.entries);
}

function compact(value: number) { return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(value); }

export function LeaderboardLive() {
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
  if (error) return <section className={dashboard.livePanel}><span className={dashboard.eyebrow}>Leaderboard</span><h2>Leaderboard is temporarily unavailable.</h2></section>;
  if (!data) return <section className={dashboard.livePanel}><span className={dashboard.eyebrow}>Leaderboard</span><h2>Loading 30-day usage ranking…</h2></section>;
  return <div className={surface.grid}>
    <div className={surface.metrics}>
      <div className={surface.metric}><span>Your rank</span><strong>{data.currentRank ? `#${data.currentRank}` : "—"}</strong></div>
      <div className={surface.metric}><span>Your 30d payload</span><strong>{compact(data.currentTokens)}</strong></div>
      <div className={surface.metric}><span>Ranked users</span><strong>{data.entries.length}</strong></div>
    </div>
    <section className={surface.card}>
      <span className={dashboard.eyebrow}>Top users · {data.windowDays} days</span>
      <h2 className={surface.title}>Estimated MCP payload</h2>
      <p className={surface.copy}>Emails are masked. This ranking reflects CodeLocal MCP payload estimates, not model-provider billing.</p>
      {data.entries.length === 0 ? <div className={surface.empty}>No MCP usage has been recorded in the last 30 days.</div> : <div className={surface.list}>{data.entries.map((entry) => <div className={surface.rankRow} key={`${entry.rank}-${entry.emailMasked}`}><span className={surface.rank}>#{entry.rank}</span><div className={surface.identity}><strong>{entry.isCurrent ? "You · " : ""}{entry.initial} · {entry.emailMasked}</strong><small>{compact(entry.tokens)} estimated tokens</small></div><span className={`${surface.badge} ${entry.isCurrent ? surface.badgeBlue : ""}`}>{entry.isCurrent ? "You" : "30d"}</span><span className={`${surface.value} ${surface.calls}`}>{entry.calls.toLocaleString()} calls</span></div>)}</div>}
    </section>
  </div>;
}
