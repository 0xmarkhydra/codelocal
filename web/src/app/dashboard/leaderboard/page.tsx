import dashboard from "../dashboard.module.css";
import { LeaderboardLive } from "./leaderboard-live";

export default function LeaderboardPage() {
  return <section className={dashboard.content}>
    <header className={dashboard.header}><div><span className={dashboard.eyebrow}>30-day usage</span><h1>Leaderboard</h1><p>Top CodeLocal users by estimated MCP payload over the last 30 days.</p></div><span className={dashboard.productionLink}>Privacy-masked</span></header>
    <LeaderboardLive />
  </section>;
}
