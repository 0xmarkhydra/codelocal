"use client";

import Link from "next/link";
import { isAccountResource } from "@/lib/contracts/account";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { AppIcon, type AppIconName } from "../app-icon";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./runtime-settings.module.css";

type SettingsDestination = {
  title: MessageKey | "Brain" | "Code Graph";
  description: MessageKey;
  href: string;
  icon: AppIconName;
};

type SettingsGroup = {
  title: MessageKey;
  items: SettingsDestination[];
};

const groups: SettingsGroup[] = [
  {
    title: "Intelligence",
    items: [
      { title: "Brain", description: "Project knowledge and context", href: "/dashboard/knowledge", icon: "brain" },
      { title: "Code Graph", description: "File, module and symbol relationships", href: "/dashboard/code-graph", icon: "connection" },
      { title: "Skills", description: "Agent capabilities", href: "/dashboard/skills", icon: "skill" },
    ],
  },
  {
    title: "Create",
    items: [
      { title: "Shots", description: "Created images and content", href: "/dashboard/shots", icon: "image" },
      { title: "Blogs", description: "Posts and series", href: "/dashboard/blogs", icon: "file" },
    ],
  },
  {
    title: "Product & access",
    items: [
      { title: "Connections", description: "Connected services and tools", href: "/dashboard/connect", icon: "connection" },
      { title: "Plugins", description: "Install and connect MCP tools", href: "/dashboard/plugins", icon: "plugin" },
      { title: "Usage", description: "Usage and limits", href: "/dashboard/usage", icon: "usage" },
      { title: "Invite", description: "Invite members to CodeLocal", href: "/dashboard/invite", icon: "invite" },
    ],
  },
  {
    title: "Account",
    items: [
      { title: "Account", description: "Account details and preferences", href: "/dashboard/account", icon: "user" },
      { title: "Security", description: "Passwords, sessions and security", href: "/dashboard/security", icon: "shield" },
    ],
  },
];

function DestinationLink({ item }: { item: SettingsDestination }) {
  const { t } = useTranslations();
  return (
    <Link className={styles.hubLink} href={item.href}>
      <span className={styles.hubIcon} aria-hidden="true"><AppIcon name={item.icon} size={17} /></span>
      <span className={styles.hubCopy}><strong>{item.title === "Brain" || item.title === "Code Graph" ? item.title : t(item.title)}</strong><small>{t(item.description)}</small></span>
      <AppIcon className={styles.hubChevron} name="chevron-right" size={15} />
    </Link>
  );
}

export function SettingsHub() {
  const { t } = useTranslations();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;

  return (
    <div className={styles.hub} aria-label={t("Settings destinations")}>
      {groups.map((group) => (
        <section className={styles.hubGroup} key={group.title}>
          <h2>{t(group.title)}</h2>
          <div className={styles.hubList}>{group.items.map((item) => <DestinationLink item={item} key={item.href} />)}</div>
        </section>
      ))}
      {readyAccount?.isAdmin ? (
        <section className={styles.hubGroup}>
          <h2>{t("Administration")}</h2>
          <div className={styles.hubList}>
            <DestinationLink item={{ title: "Admin", description: "System and tenant administration", href: "/dashboard/admin", icon: "admin" }} />
          </div>
        </section>
      ) : null}
    </div>
  );
}
