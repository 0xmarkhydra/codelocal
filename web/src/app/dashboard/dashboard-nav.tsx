"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { AppIcon, type AppIconName } from "./app-icon";
import styles from "./dashboard-chrome.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

type NavigationItem = { label: string; href: string; icon: AppIconName };
const groups: { label: string; items: NavigationItem[] }[] = [
  {
    label: "WORKSPACE",
    items: [
      { label: "Tổng quan", href: "/dashboard", icon: "module" },
      { label: "Dự án", href: "/dashboard/workspaces", icon: "folder" },
      { label: "Design", href: "/dashboard/design", icon: "edit" },
      { label: "Thiết bị", href: "/dashboard/devices", icon: "device" },
      { label: "Kết nối MCP", href: "/dashboard/connect", icon: "connection" },
    ],
  },
  {
    label: "INTELLIGENCE",
    items: [
      { label: "Project Brain", href: "/dashboard/knowledge", icon: "brain" },
      { label: "Code Graph", href: "/dashboard/code-graph", icon: "code" },
      { label: "Skills", href: "/dashboard/skills", icon: "skill" },
      { label: "Plugins", href: "/dashboard/plugins", icon: "plugin" },
      { label: "Mức sử dụng", href: "/dashboard/usage", icon: "usage" },
    ],
  },
];

function NavLink({
  item,
  pathname,
}: {
  item: NavigationItem;
  pathname: string;
}) {
  const active =
    pathname === item.href ||
    (item.href !== "/dashboard" && pathname.startsWith(`${item.href}/`));
  return (
    <Link
      className={`${styles.navLink} ${active ? styles.activeNav : ""}`}
      href={item.href}
      aria-current={active ? "page" : undefined}
    >
      <AppIcon name={item.icon} size={18} />
      <span>{item.label}</span>
      {active && <i aria-hidden="true" />}
    </Link>
  );
}

export function DashboardNav({
  onNavigate,
  children,
  compact = false,
}: { onNavigate?: () => void; children?: ReactNode; compact?: boolean } = {}) {
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount =
    account.state.kind === "ready" ? account.state.value : undefined;
  return (
    <>
      <nav
        className={styles.nav}
        aria-label="Dashboard navigation"
        onClick={(event) => {
          if ((event.target as HTMLElement).closest("a")) onNavigate?.();
        }}
      >
        {(compact ? groups.slice(0, 1) : groups).map((group) => (
          <div className={styles.navGroup} key={group.label}>
            <span className={styles.navLabel}>{group.label}</span>
            {group.items.map((item) => (
              <NavLink item={item} pathname={pathname} key={item.href} />
            ))}
          </div>
        ))}
        <div className={styles.navGroup}>
          <span className={styles.navLabel}>QUẢN LÝ</span>
          <NavLink
            item={{
              label: "Cài đặt",
              href: "/dashboard/settings",
              icon: "settings",
            }}
            pathname={pathname}
          />
          <NavLink
            item={{
              label: "Bảo mật",
              href: "/dashboard/security",
              icon: "shield",
            }}
            pathname={pathname}
          />
          {readyAccount?.isAdmin && (
            <NavLink
              item={{
                label: "Quản trị",
                href: "/dashboard/admin",
                icon: "admin",
              }}
              pathname={pathname}
            />
          )}
        </div>
      </nav>
      {children}
      <Link
        className={styles.desktopCard}
        href="/#desktop"
        onClick={onNavigate}
      >
        <span>
          <AppIcon name="device" size={19} />
          <small>SẮP RA MẮT</small>
        </span>
        <strong>
          CodeLocal Desktop <span aria-hidden="true">↗</span>
        </strong>
        <p>Ngôi nhà mới cho AI chat.</p>
      </Link>
      {readyAccount ? (
        <details className={styles.account}>
          <summary>
            <span className={styles.avatar} aria-hidden="true">
              {readyAccount.email.slice(0, 1).toUpperCase()}
            </span>
            <span className={styles.accountCopy}>
              <strong title={readyAccount.email}>{readyAccount.email}</strong>
              <small>
                {readyAccount.isAdmin ? "Admin" : "Tài khoản cá nhân"}
              </small>
            </span>
            <AppIcon name="chevron-down" size={14} />
          </summary>
          <div className={styles.accountMenu}>
            <Link href="/dashboard/account" onClick={onNavigate}>
              Tài khoản
            </Link>
            <form method="post" action="/logout">
              <input type="hidden" name="csrf" value={readyAccount.csrf} />
              <input type="hidden" name="next" value="/dashboard" />
              <button type="submit">Đăng xuất</button>
            </form>
          </div>
        </details>
      ) : (
        <Link
          className={styles.accountFallback}
          href={
            account.state.kind === "unauthenticated"
              ? "/login?next=%2Fdashboard"
              : "/dashboard/account"
          }
        >
          <AppIcon name="user" size={17} />
          {account.state.kind === "unauthenticated" ? "Đăng nhập" : "Tài khoản"}
        </Link>
      )}
    </>
  );
}
