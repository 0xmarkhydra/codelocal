"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { isAccountResource } from "@/lib/contracts/account";
import styles from "./dashboard.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

const navigation = [
  { label: "Overview", href: "/dashboard" },
  { label: "Workspaces", href: "/dashboard/workspaces" },
  { label: "Knowledge Graph", href: "/dashboard/knowledge" },
  { label: "Code Graph", href: "/dashboard/code-graph" },
  { label: "Devices", href: "/dashboard/devices" },
  { label: "Usage", href: "/dashboard/usage" },
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
  const items = account.state.kind === "ready" && account.state.value.isAdmin
    ? [...navigation, { label: "Admin", href: "/dashboard/admin" }]
    : navigation;
  return (
    <nav className={styles.nav} aria-label="Dashboard navigation">
      {items.map((item) => {
        const active = isActive(pathname, item.href);
        return (
          <Link className={active ? styles.activeNav : undefined} href={item.href} aria-current={active ? "page" : undefined} key={item.href}>
            <i aria-hidden="true" />
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
