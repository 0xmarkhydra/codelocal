"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { isAccountResource } from "@/lib/contracts/account";
import type { AppIconName } from "./app-icon";
import { AppIcon } from "./app-icon";
import styles from "./dashboard.module.css";
import controls from "./dashboard-controls.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

type NavigationItem = { label: string; href: string; icon: AppIconName };

const navigation: NavigationItem[] = [
  { label: "Chat", href: "/dashboard", icon: "chat" },
  { label: "Shots", href: "/dashboard/shots", icon: "image" },
  { label: "Blogs", href: "/dashboard/blogs", icon: "file" },
  { label: "Brain", href: "/dashboard/knowledge", icon: "brain" },
  { label: "Skills", href: "/dashboard/skills", icon: "skill" },
  { label: "Connections", href: "/dashboard/connect", icon: "connection" },
  { label: "Workspaces", href: "/dashboard/workspaces", icon: "folder" },
  { label: "Devices", href: "/dashboard/devices", icon: "device" },
  { label: "Usage", href: "/dashboard/usage", icon: "usage" },
  { label: "Settings", href: "/dashboard/settings", icon: "settings" },
  { label: "Invite", href: "/dashboard/invite", icon: "invite" },
];

function isActive(pathname: string, href: string) {
  if (href === "/dashboard") return pathname === href;
  if (href === "/dashboard/knowledge") {
    return pathname.startsWith("/dashboard/knowledge") || pathname.startsWith("/dashboard/code-graph");
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

function NavLink({ item, pathname }: { item: NavigationItem; pathname: string }) {
  const active = isActive(pathname, item.href);
  return (
    <Link className={active ? styles.activeNav : undefined} href={item.href} aria-current={active ? "page" : undefined}>
      <AppIcon className={styles.navIcon} name={item.icon} size={18} />
      <span>{item.label}</span>
    </Link>
  );
}

export function DashboardNav() {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const items = readyAccount?.isAdmin
    ? [...navigation, { label: "Admin", href: "/dashboard/admin", icon: "admin" as const }]
    : navigation;

  return (
    <>
      <nav className={styles.nav} aria-label="Dashboard navigation">
        <div className={styles.navPrimary}>
          {items.map((item) => <NavLink item={item} pathname={pathname} key={item.href} />)}
        </div>
      </nav>

      {readyAccount && (
        <details className={controls.sidebarAccount}>
          <summary className={controls.sidebarAccountSummary}>
            <span className={controls.sidebarAvatar} aria-hidden="true">{readyAccount.email.slice(0, 1).toUpperCase()}</span>
            <span className={controls.sidebarAccountCopy}>
              <strong title={readyAccount.email}>{readyAccount.email}</strong>
              <span>{readyAccount.isAdmin ? "Admin" : "Account"}</span>
            </span>
            <AppIcon className={controls.sidebarChevron} name="chevron-down" size={14} />
          </summary>
          <div className={controls.sidebarAccountMenu}>
            <Link href="/dashboard/account">Account</Link>
            <Link href="/dashboard/security">Security</Link>
            <form method="post" action="/logout">
              <input type="hidden" name="csrf" value={readyAccount.csrf} />
              <input type="hidden" name="next" value="/dashboard" />
              <button className={controls.signOutButton} type="submit">Sign out</button>
            </form>
          </div>
        </details>
      )}
    </>
  );
}
