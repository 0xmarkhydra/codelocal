"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import styles from "./dashboard.module.css";

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
  return (
    <nav className={styles.nav} aria-label="Dashboard navigation">
      {navigation.map((item) => {
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
