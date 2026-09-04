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
type NavigationGroup = { label: string; icon: AppIconName; items: NavigationItem[] };

const chat: NavigationItem = { label: "Chat", href: "/dashboard", icon: "chat" };
const intelligence: NavigationGroup = {
  label: "Intelligence", icon: "brain", items: [
    { label: "Brain", href: "/dashboard/knowledge", icon: "brain" },
    { label: "Code Graph", href: "/dashboard/code-graph", icon: "connection" },
    { label: "Skills", href: "/dashboard/skills", icon: "skill" },
  ],
};
const primary: NavigationItem[] = [
  { label: "Workspaces", href: "/dashboard/workspaces", icon: "folder" },
  { label: "Devices", href: "/dashboard/devices", icon: "device" },
];
const create: NavigationGroup = { label: "Create", icon: "file", items: [
  { label: "Shots", href: "/dashboard/shots", icon: "image" },
  { label: "Blogs", href: "/dashboard/blogs", icon: "file" },
] };
const more: NavigationGroup = { label: "More", icon: "settings", items: [
  { label: "Connections", href: "/dashboard/connect", icon: "connection" },
  { label: "Usage", href: "/dashboard/usage", icon: "usage" },
  { label: "Settings", href: "/dashboard/settings", icon: "settings" },
  { label: "Invite", href: "/dashboard/invite", icon: "invite" },
] };

function isActive(pathname: string, href: string) {
  if (href === "/dashboard") return pathname === href;
  if (href === "/dashboard/knowledge") return pathname.startsWith(href);
  return pathname === href || pathname.startsWith(`${href}/`);
}

function NavLink({ item, pathname, onNavigate }: { item: NavigationItem; pathname: string; onNavigate?: () => void }) {
  const active = isActive(pathname, item.href);
  return <Link onClick={onNavigate} className={active ? styles.activeNav : undefined} href={item.href} aria-current={active ? "page" : undefined}>
    <AppIcon className={styles.navIcon} name={item.icon} size={18} /><span>{item.label}</span>
  </Link>;
}

function NavGroup({ group, pathname, mobileMain = false }: { group: NavigationGroup; pathname: string; mobileMain?: boolean }) {
  const active = group.items.some((item) => isActive(pathname, item.href));
  return <details className={`${styles.navDisclosure} ${mobileMain ? styles.mobileMainGroup : ""}`} open={active || undefined}>
    <summary className={active ? styles.navGroupActive : undefined}>
      <AppIcon className={styles.navIcon} name={group.icon} size={18} /><span>{group.label}</span>
      <AppIcon className={styles.navChevron} name="chevron-down" size={13} />
    </summary>
    <div className={styles.navDisclosureLinks}>{group.items.map((item) => <NavLink key={item.href} item={item} pathname={pathname} />)}</div>
  </details>;
}

export function DashboardNav({ onNavigate }: { onNavigate?: () => void } = {}) {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const moreItems = readyAccount?.isAdmin ? [...more.items, { label: "Admin", href: "/dashboard/admin", icon: "admin" as const }] : more.items;
  return <>
    <nav className={styles.nav} aria-label="Dashboard navigation" onClick={(event) => { if ((event.target as HTMLElement).closest("a")) onNavigate?.(); }}>
      <div className={styles.navPrimary}>
        <NavLink item={chat} pathname={pathname} />
        <NavGroup group={intelligence} pathname={pathname} mobileMain />
        {primary.map((item) => <NavLink item={item} pathname={pathname} key={item.href} />)}
        <NavGroup group={create} pathname={pathname} />
        <NavGroup group={{ ...more, items: moreItems }} pathname={pathname} mobileMain />
      </div>
    </nav>
    {readyAccount && <details className={controls.sidebarAccount}>
      <summary className={controls.sidebarAccountSummary}><span className={controls.sidebarAvatar} aria-hidden="true">{readyAccount.email.slice(0, 1).toUpperCase()}</span><span className={controls.sidebarAccountCopy}><strong title={readyAccount.email}>{readyAccount.email}</strong><span>{readyAccount.isAdmin ? "Admin" : "Account"}</span></span><AppIcon className={controls.sidebarChevron} name="chevron-down" size={14} /></summary>
      <div className={controls.sidebarAccountMenu}><Link href="/dashboard/account">Account</Link><Link href="/dashboard/security">Security</Link><form method="post" action="/logout"><input type="hidden" name="csrf" value={readyAccount.csrf} /><input type="hidden" name="next" value="/dashboard" /><button className={controls.signOutButton} type="submit">Sign out</button></form></div>
    </details>}
  </>;
}
