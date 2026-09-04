import { DashboardSidebar } from "./dashboard-sidebar";
import styles from "./dashboard.module.css";

export default function DashboardLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <main className={styles.shell}>
      <DashboardSidebar />
      {children}
    </main>
  );
}

