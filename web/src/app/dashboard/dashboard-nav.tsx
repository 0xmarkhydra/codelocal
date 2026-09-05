"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import type { AppIconName } from "./app-icon";
import { AppIcon } from "./app-icon";
import styles from "./dashboard.module.css";
import controls from "./dashboard-controls.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

type NavigationItem = { label: string; href: string; icon: AppIconName };

const chat: NavigationItem = { label: "New task", href: "/dashboard", icon: "plus" };
const utilities: NavigationItem[] = [
  { label: "Devices", href: "/dashboard/devices", icon: "device" },
  { label: "Settings", href: "/dashboard/settings", icon: "settings" },
];

const settingsRoutes = [
  "/dashboard/settings",
  "/dashboard/knowledge",
  "/dashboard/code-graph",
  "/dashboard/skills",
  "/dashboard/shots",
  "/dashboard/blogs",
  "/dashboard/connect",
  "/dashboard/plugins",
  "/dashboard/usage",
  "/dashboard/invite",
  "/dashboard/account",
  "/dashboard/security",
  "/dashboard/admin",
];

function pathMatches(pathname: string, href: string) {
  return pathname === href || pathname.startsWith(`${href}/`);
}

function isActive(pathname: string, href: string) {
  if (href === "/dashboard") return pathname === href;
  if (href === "/dashboard/settings") return settingsRoutes.some((route) => pathMatches(pathname, route));
  return pathMatches(pathname, href);
}

function NavLink({ item, pathname }: { item: NavigationItem; pathname: string }) {
  const active = isActive(pathname, item.href);
  return <Link className={active ? styles.activeNav : undefined} href={item.href} aria-current={active ? "page" : undefined}>
    <AppIcon className={styles.navIcon} name={item.icon} size={18} /><span>{item.label}</span>
  </Link>;
}

export function DashboardNav({ onNavigate, children, compact = false }: { onNavigate?: () => void; children?: ReactNode; compact?: boolean } = {}) {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const projectItems = workspaces.state.kind === "ready" ? workspaces.state.value.items : [];
  return <>
    <nav className={`${styles.nav} ${compact ? styles.navCompact : ""}`} aria-label="Dashboard navigation" onClick={(event) => { if ((event.target as HTMLElement).closest("a")) onNavigate?.(); }}>
      <div className={styles.navPrimary}>
        <NavLink item={chat} pathname={pathname} />
        <div className={styles.navSectionLabel}>Projects</div>
        <div className={styles.navProjects}>
          {projectItems.slice(0, 8).map((workspace) => (
            <Link className={styles.navProject} href={`/dashboard?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`} key={`${workspace.deviceId}:${workspace.workspaceId}`}>
              <span className={styles.navProjectFolder} aria-hidden="true"><AppIcon name="folder" size={15} /></span>
              <span title={workspace.workspaceName}>{workspace.workspaceName}</span>
              <i data-state={workspace.status} aria-label={workspace.status} />
            </Link>
          ))}
          <Link className={styles.navAllProjects} href="/dashboard/workspaces"><AppIcon name="chevron-right" size={13} />All projects</Link>
        </div>
        <div className={styles.navUtilities}>
          {utilities.map((item) => <NavLink item={item} pathname={pathname} key={item.href} />)}
        </div>
      </div>
    </nav>
    {children}
    {readyAccount && <details className={controls.sidebarAccount}>
      <summary className={controls.sidebarAccountSummary}><span className={controls.sidebarAvatar} aria-hidden="true">{readyAccount.email.slice(0, 1).toUpperCase()}</span><span className={controls.sidebarAccountCopy}><strong title={readyAccount.email}>{readyAccount.email}</strong><span>{readyAccount.isAdmin ? "Admin" : "Account"}</span></span><AppIcon className={controls.sidebarChevron} name="chevron-down" size={14} /></summary>
      <div className={controls.sidebarAccountMenu}><Link href="/dashboard/account">Account</Link><Link href="/dashboard/security">Security</Link><form method="post" action="/logout"><input type="hidden" name="csrf" value={readyAccount.csrf} /><input type="hidden" name="next" value="/dashboard" /><button className={controls.signOutButton} type="submit">Sign out</button></form></div>
    </details>}
  </>;
}
