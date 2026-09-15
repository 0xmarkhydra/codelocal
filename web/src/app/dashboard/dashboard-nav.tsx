"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { AppIcon, type AppIconName } from "./app-icon";
import styles from "./dashboard-chrome.module.css";
import { useDashboardResource } from "./use-dashboard-resource";
import { LanguageSelect, useTranslations } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";

type NavigationItem = { label: MessageKey; href: string; icon: AppIconName };
const groups: { label: MessageKey; items: NavigationItem[] }[] = [
  {
    label: "Workspace",
    items: [
      { label: "Overview", href: "/dashboard", icon: "module" },
      { label: "Projects", href: "/dashboard/workspaces", icon: "folder" },
      { label: "Design", href: "/dashboard/design", icon: "edit" },
      { label: "Devices", href: "/dashboard/devices", icon: "device" },
      { label: "MCP connections", href: "/dashboard/connect", icon: "connection" },
    ],
  },
  {
    label: "Intelligence",
    items: [
      { label: "Project Brain", href: "/dashboard/knowledge", icon: "brain" },
      { label: "Code Graph", href: "/dashboard/code-graph", icon: "code" },
      { label: "Skills", href: "/dashboard/skills", icon: "skill" },
      { label: "Plugins", href: "/dashboard/plugins", icon: "plugin" },
      { label: "Usage", href: "/dashboard/usage", icon: "usage" },
    ],
  },
  {
    label: "Community",
    items: [
      { label: "Forums", href: "/dashboard/forums", icon: "forum" },
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
  const { t } = useTranslations();
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
      <span>{t(item.label)}</span>
      {active && <i aria-hidden="true" />}
    </Link>
  );
}

export function DashboardNav({
  onNavigate,
  children,
  compact = false,
}: { onNavigate?: () => void; children?: ReactNode; compact?: boolean } = {}) {
  const { t } = useTranslations();
  const pathname = usePathname();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount =
    account.state.kind === "ready" ? account.state.value : undefined;
  return (
    <>
      <nav
        className={styles.nav}
        aria-label={t("Dashboard navigation")}
        onClick={(event) => {
          if ((event.target as HTMLElement).closest("a")) onNavigate?.();
        }}
      >
        {(compact ? groups.slice(0, 1) : groups).map((group) => (
          <div className={styles.navGroup} key={group.label}>
            <span className={styles.navLabel}>{t(group.label)}</span>
            {group.items.map((item) => (
              <NavLink item={item} pathname={pathname} key={item.href} />
            ))}
          </div>
        ))}
        <div className={styles.navGroup}>
          <span className={styles.navLabel}>{t("Management")}</span>
          <NavLink
            item={{
              label: "Settings",
              href: "/dashboard/settings",
              icon: "settings",
            }}
            pathname={pathname}
          />
          <NavLink
            item={{
              label: "Security",
              href: "/dashboard/security",
              icon: "shield",
            }}
            pathname={pathname}
          />
          {readyAccount?.isAdmin && (
            <NavLink
              item={{
                label: "Administration",
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
          <small>{t("Coming soon")}</small>
        </span>
        <strong>
          CodeLocal Desktop <span aria-hidden="true">↗</span>
        </strong>
        <p>{t("A new home for AI chat.")}</p>
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
                {readyAccount.isAdmin ? t("Admin") : t("Personal account")}
              </small>
            </span>
            <AppIcon name="chevron-down" size={14} />
          </summary>
          <div className={styles.accountMenu}>
            <LanguageSelect />
            <Link href="/dashboard/account" onClick={onNavigate}>
              {t("Account")}
            </Link>
            <form method="post" action="/logout">
              <input type="hidden" name="csrf" value={readyAccount.csrf} />
              <input type="hidden" name="next" value="/dashboard" />
              <button type="submit">{t("Sign out")}</button>
            </form>
          </div>
        </details>
      ) : (
        <><LanguageSelect /><Link
          className={styles.accountFallback}
          href={
            account.state.kind === "unauthenticated"
              ? "/login?next=%2Fdashboard"
              : "/dashboard/account"
          }
        >
          <AppIcon name="user" size={17} />
          {account.state.kind === "unauthenticated" ? t("Sign in") : t("Account")}
        </Link></>
      )}
    </>
  );
}
