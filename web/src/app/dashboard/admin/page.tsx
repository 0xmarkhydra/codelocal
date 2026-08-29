import dashboard from "../dashboard.module.css";
import { AdminLive } from "./admin-live";

export default function AdminPage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}>
        <div>
          <span className={dashboard.eyebrow}>Admin</span>
          <h1>Admin Dashboard</h1>
          <p>Quản lý người dùng và theo dõi trạng thái hệ thống từ dữ liệu thật.</p>
        </div>
        <span className={dashboard.adminBadge}>ADMIN</span>
      </header>
      <AdminLive />
    </section>
  );
}
