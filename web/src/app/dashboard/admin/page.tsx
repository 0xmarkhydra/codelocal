import dashboard from "../dashboard.module.css";
import { AdminLive } from "./admin-live";

export default function AdminPage() {
  return <section className={dashboard.content}>
    <header className={dashboard.header}><div><span className={dashboard.eyebrow}>Administration</span><h1>User network</h1><p>Live runtime status, MCP activity and referral relationships without exposing local source code.</p></div><span className={dashboard.productionLink}>Admin only</span></header>
    <AdminLive />
  </section>;
}
