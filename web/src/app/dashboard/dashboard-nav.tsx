"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { isAccountResource } from "@/lib/contracts/account";
import styles from "./dashboard.module.css";
import controls from "./dashboard-controls.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

type NavigationItem = { label: string; href: string };

const navigation: NavigationItem[] = [
  { label: "Overview", href: "/dashboard" },
  { label: "MCP Connections", href: "/dashboard/connect" },
  { label: "Workspaces", href: "/dashboard/workspaces" },
  { label: "Knowledge Graph", href: "/dashboard/knowledge" },
  { label: "Code Graph", href: "/dashboard/code-graph" },
  { label: "Devices", href: "/dashboard/devices" },
  { label: "Usage", href: "/dashboard/usage" },
  { label: "Invite", href: "/dashboard/invite" },
  { label: "Leaderboard", href: "/dashboard/leaderboard" },
  { label: "Security", href: "/dashboard/security" },
  { label: "Account", href: "/dashboard/account" },
];

function isActive(pathname: string, href: string) {
  if (href === "/dashboard") return pathname === href;
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function DashboardNav() {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const items: NavigationItem[] = readyAccount?.isAdmin
    ? [...navigation, { label: "Admin", href: "/dashboard/admin" }]
    : navigation;

  return (
    <>
      <nav className={styles.nav} aria-label="Dashboard navigation">
        {items.map((item) => {
          const active = isActive(pathname, item.href);
          return (
            <Link className={active ? styles.activeNav : undefined} href={item.href} aria-current={active ? "page" : undefined} key={item.href}>
              {item.label}
            </Link>
          );
        })}
      </nav>

      {readyAccount && (
        <div className={controls.sidebarAccount}>
          <div className={controls.sidebarAccountCopy}>
            <span>Signed in</span>
            <strong title={readyAccount.email}>{readyAccount.email}</strong>
          </div>
          <form method="post" action="/logout">
            <input type="hidden" name="csrf" value={readyAccount.csrf} />
            <input type="hidden" name="next" value="/dashboard" />
            <button className={controls.signOutButton} type="submit">Sign out</button>
          </form>
        </div>
      )}
    </>
  );
}
