"use client";

import { usePathname } from "next/navigation";
import { DashboardSidebar } from "./dashboard-sidebar";
import styles from "./dashboard.module.css";

const dashboardRouteTitles: Record<string, string> = {
  "/dashboard/workspaces": "Projects",
  "/dashboard/design": "Design",
  "/dashboard/devices": "Devices",
  "/dashboard/settings": "Settings",
  "/dashboard/knowledge": "Knowledge",
  "/dashboard/code-graph": "Code Graph",
  "/dashboard/skills": "Skills",
  "/dashboard/usage": "Usage",
  "/dashboard/connect": "Connections",
  "/dashboard/shots": "Shots",
  "/dashboard/blogs": "Blogs",
  "/dashboard/invite": "Invite",
  "/dashboard/account": "Account",
  "/dashboard/security": "Security",
  "/dashboard/admin": "Admin",
  "/dashboard/plugins": "Plugins",
  "/dashboard/leaderboard": "Leaderboard",
};

function dashboardRouteTitle(pathname: string) {
  return dashboardRouteTitles[pathname]
    ?? Object.entries(dashboardRouteTitles).find(([route]) => pathname.startsWith(`${route}/`))?.[1]
    ?? "CodeLocal";
}

export function DashboardShell({ children }: Readonly<{ children: React.ReactNode }>) {
  const pathname = usePathname();
  const chatRoute = pathname === "/dashboard";

  return (
    <main className={`${styles.shell} ${chatRoute ? styles.chatShellLayout : styles.dashboardShellLayout}`}>
      {!chatRoute ? <DashboardSidebar title={dashboardRouteTitle(pathname)} /> : null}
      {children}
    </main>
  );
}
