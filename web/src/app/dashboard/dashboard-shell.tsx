"use client";

import { usePathname } from "next/navigation";
import { DashboardSidebar } from "./dashboard-sidebar";
import styles from "./dashboard.module.css";

export function DashboardShell({ children }: Readonly<{ children: React.ReactNode }>) {
  const pathname = usePathname();
  const chatRoute = pathname === "/dashboard";

  return (
    <main className={`${styles.shell} ${chatRoute ? styles.chatShellLayout : ""}`}>
      {!chatRoute ? <DashboardSidebar /> : null}
      {children}
    </main>
  );
}
