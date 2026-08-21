"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { isAccountResource } from "@/lib/contracts/account";
import styles from "./dashboard.module.css";
import controls from "./dashboard-controls.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

type IconName = "home" | "link" | "folder" | "device" | "book" | "graph" | "usage" | "invite" | "admin";
type NavigationItem = { label: string; href: string; icon: IconName };
type NavigationGroup = { label: string; items: NavigationItem[] };

const primaryNavigation: NavigationItem[] = [
  { label: "Home", href: "/dashboard", icon: "home" },
  { label: "Connections", href: "/dashboard/connect", icon: "link" },
  { label: "Workspaces", href: "/dashboard/workspaces", icon: "folder" },
  { label: "Devices", href: "/dashboard/devices", icon: "device" },
];

const intelligenceNavigation: NavigationGroup = {
  label: "Intelligence",
  items: [
    { label: "Knowledge", href: "/dashboard/knowledge", icon: "book" },
    { label: "Code Graph", href: "/dashboard/code-graph", icon: "graph" },
  ],
};

const moreNavigation: NavigationItem[] = [
  { label: "Usage", href: "/dashboard/usage", icon: "usage" },
  { label: "Invite", href: "/dashboard/invite", icon: "invite" },
];

function isActive(pathname: string, href: string) {
  if (href === "/dashboard") return pathname === href;
  return pathname === href || pathname.startsWith(`${href}/`);
}

function NavIcon({ name }: { name: IconName }) {
  const common = { fill: "none", stroke: "currentColor", strokeWidth: 1.8, strokeLinecap: "round" as const, strokeLinejoin: "round" as const };
  switch (name) {
    case "home":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><path d="m3 11 9-8 9 8" /><path d="M5 10v10h14V10M9 20v-6h6v6" /></svg>;
    case "link":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><path d="M10 13a5 5 0 0 0 7.1.1l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1" /><path d="M14 11a5 5 0 0 0-7.1-.1l-2 2A5 5 0 0 0 12 20l1.1-1.1" /></svg>;
    case "folder":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><path d="M3 6.5A2.5 2.5 0 0 1 5.5 4H10l2 2.5h6.5A2.5 2.5 0 0 1 21 9v7.5a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3z" /></svg>;
    case "device":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><rect x="3" y="4" width="18" height="13" rx="2" /><path d="M8 21h8M12 17v4" /></svg>;
    case "book":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><path d="M4 5.5A3.5 3.5 0 0 1 7.5 2H12v18H7.5A3.5 3.5 0 0 0 4 23z" /><path d="M20 5.5A3.5 3.5 0 0 0 16.5 2H12v18h4.5A3.5 3.5 0 0 1 20 23z" /></svg>;
    case "graph":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><circle cx="12" cy="5" r="2.5" /><circle cx="5" cy="18" r="2.5" /><circle cx="19" cy="18" r="2.5" /><path d="m10.8 7.2-4.5 8.6m6.9-8.6 4.5 8.6M7.5 18h9" /></svg>;
    case "usage":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><path d="M4 20V10m6 10V4m6 16v-7m4 7H2" /></svg>;
    case "invite":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><circle cx="9" cy="8" r="4" /><path d="M2.5 21a6.5 6.5 0 0 1 13 0M19 8v6m-3-3h6" /></svg>;
    case "admin":
      return <svg className={styles.navIcon} viewBox="0 0 24 24" aria-hidden="true" {...common}><path d="M12 3 4 7v5c0 5 3.4 8.1 8 9 4.6-.9 8-4 8-9V7z" /><path d="M9 12h6m-3-3v6" /></svg>;
  }
}

function NavLink({ item, pathname }: { item: NavigationItem; pathname: string }) {
  const active = isActive(pathname, item.href);
  return (
    <Link className={active ? styles.activeNav : undefined} href={item.href} aria-current={active ? "page" : undefined}>
      <NavIcon name={item.icon} />
      <span>{item.label}</span>
    </Link>
  );
}

function DisclosureGroup({ group, pathname }: { group: NavigationGroup; pathname: string }) {
  const containsActive = group.items.some((item) => isActive(pathname, item.href));
  return (
    <details className={styles.navDisclosure} open={containsActive || undefined} key={`${group.label}:${pathname}`}>
      <summary>
        <span>{group.label}</span>
        <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m5 6 3 3 3-3" /></svg>
      </summary>
      <div className={styles.navDisclosureLinks}>
        {group.items.map((item) => <NavLink item={item} pathname={pathname} key={item.href} />)}
      </div>
    </details>
  );
}

export function DashboardNav() {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const moreItems = readyAccount?.isAdmin
    ? [...moreNavigation, { label: "User network", href: "/dashboard/admin", icon: "admin" as const }]
    : moreNavigation;

  return (
    <>
      <nav className={styles.nav} aria-label="Dashboard navigation">
        <div className={styles.navPrimary}>
          {primaryNavigation.map((item) => <NavLink item={item} pathname={pathname} key={item.href} />)}
        </div>
        <DisclosureGroup group={intelligenceNavigation} pathname={pathname} />
        <DisclosureGroup group={{ label: "More", items: moreItems }} pathname={pathname} />
      </nav>

      {readyAccount && (
        <details className={controls.sidebarAccount}>
          <summary className={controls.sidebarAccountSummary}>
            <span className={controls.sidebarAvatar} aria-hidden="true">{readyAccount.email.slice(0, 1).toUpperCase()}</span>
            <span className={controls.sidebarAccountCopy}>
              <strong title={readyAccount.email}>{readyAccount.email}</strong>
              <span>Account</span>
            </span>
            <span className={controls.sidebarChevron} aria-hidden="true">⌄</span>
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
