import dashboard from "../dashboard.module.css";
import { AdminLive } from "./admin-live";

export default function AdminPage() {
  return <section className={dashboard.content}>
    <header className={dashboard.header}><div><h1>User network</h1></div><span className={dashboard.productionLink}>Admin</span></header>
    <AdminLive />
  </section>;
}
