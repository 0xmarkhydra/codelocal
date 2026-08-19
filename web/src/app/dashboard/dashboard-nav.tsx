"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { isAccountResource } from "@/lib/contracts/account";
import styles from "./dashboard.module.css";
import controls from "./dashboard-controls.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

type NavigationItem = {
  label: string;
  href: string;
  owner: "next" | "go";
};

const navigation: NavigationItem[] = [
  { label: "Overview", href: "/dashboard", owner: "next" },
  { label: "MCP Connections", href: "/dashboard/connect", owner: "go" },
  { label: "Workspaces", href: "/dashboard/workspaces", owner: "next" },
  { label: "Knowledge Graph", href: "/dashboard/knowledge", owner: "next" },
  { label: "Code Graph", href: "/dashboard/code-graph", owner: "next" },
  { label: "Devices", href: "/dashboard/devices", owner: "next" },
  { label: "Usage", href: "/dashboard/usage", owner: "next" },
  { label: "Invite", href: "/dashboard/invite", owner: "go" },
  { label: "Leaderboard", href: "/dashboard/leaderboard", owner: "go" },
  { label: "Security", href: "/dashboard/security", owner: "next" },
  { label: "Account", href: "/dashboard/account", owner: "next" },
];

function isActive(pathname: string, href: string) {
  if (href === "/dashboard") return pathname === href;
  return pathname === href || pathname.startsWith(`${href}/`);
}

function NavigationLink({ item, active }: { item: NavigationItem; active: boolean }) {
  const className = active ? styles.activeNav : undefined;
  const content = (
    <>
      <i aria-hidden="true" />
      {item.label}
    </>
  );
  if (item.owner === "go") {
    return <a className={className} href={item.href} aria-current={active ? "page" : undefined}>{content}</a>;
  }
  return <Link className={className} href={item.href} aria-current={active ? "page" : undefined}>{content}</Link>;
}

export function DashboardNav() {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const items: NavigationItem[] = readyAccount?.isAdmin
    ? [...navigation, { label: "Admin", href: "/dashboard/admin", owner: "go" }]
    : navigation;

  return (
    <>
      <nav className={styles.nav} aria-label="Dashboard navigation">
        {items.map((item) => (
          <NavigationLink item={item} active={isActive(pathname, item.href)} key={item.href} />
        ))}
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
