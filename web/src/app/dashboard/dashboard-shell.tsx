import { DashboardSidebar } from "./dashboard-sidebar";
import styles from "./dashboard-chrome.module.css";

export function DashboardShell({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <main className={styles.shell}>
      <DashboardSidebar />
      {children}
    </main>
  );
}
