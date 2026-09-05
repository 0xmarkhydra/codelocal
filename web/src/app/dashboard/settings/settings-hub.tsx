"use client";

import Link from "next/link";
import { isAccountResource } from "@/lib/contracts/account";
import { AppIcon, type AppIconName } from "../app-icon";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./runtime-settings.module.css";

type SettingsDestination = {
  title: string;
  description: string;
  href: string;
  icon: AppIconName;
};

type SettingsGroup = {
  title: string;
  items: SettingsDestination[];
};

const groups: SettingsGroup[] = [
  {
    title: "Intelligence",
    items: [
      { title: "Brain", description: "Knowledge và context của project", href: "/dashboard/knowledge", icon: "brain" },
      { title: "Code Graph", description: "Quan hệ giữa file, module và symbol", href: "/dashboard/code-graph", icon: "connection" },
      { title: "Skills", description: "Khả năng mở rộng của agent", href: "/dashboard/skills", icon: "skill" },
    ],
  },
  {
    title: "Create",
    items: [
      { title: "Shots", description: "Ảnh và nội dung đã tạo", href: "/dashboard/shots", icon: "image" },
      { title: "Blogs", description: "Bài viết và series", href: "/dashboard/blogs", icon: "file" },
    ],
  },
  {
    title: "Product & access",
    items: [
      { title: "Connections", description: "Kết nối dịch vụ và công cụ", href: "/dashboard/connect", icon: "connection" },
      { title: "Usage", description: "Mức sử dụng và giới hạn", href: "/dashboard/usage", icon: "usage" },
      { title: "Invite", description: "Mời thành viên vào CodeLocal", href: "/dashboard/invite", icon: "invite" },
    ],
  },
  {
    title: "Account",
    items: [
      { title: "Account", description: "Thông tin và tùy chọn tài khoản", href: "/dashboard/account", icon: "user" },
      { title: "Security", description: "Mật khẩu, phiên và bảo mật", href: "/dashboard/security", icon: "shield" },
    ],
  },
];

function DestinationLink({ item }: { item: SettingsDestination }) {
  return (
    <Link className={styles.hubLink} href={item.href}>
      <span className={styles.hubIcon} aria-hidden="true"><AppIcon name={item.icon} size={17} /></span>
      <span className={styles.hubCopy}><strong>{item.title}</strong><small>{item.description}</small></span>
      <AppIcon className={styles.hubChevron} name="chevron-right" size={15} />
    </Link>
  );
}

export function SettingsHub() {
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;

  return (
    <div className={styles.hub} aria-label="Settings destinations">
      {groups.map((group) => (
        <section className={styles.hubGroup} key={group.title}>
          <h2>{group.title}</h2>
          <div className={styles.hubList}>{group.items.map((item) => <DestinationLink item={item} key={item.href} />)}</div>
        </section>
      ))}
      {readyAccount?.isAdmin ? (
        <section className={styles.hubGroup}>
          <h2>Administration</h2>
          <div className={styles.hubList}>
            <DestinationLink item={{ title: "Admin", description: "Quản trị hệ thống và tenant", href: "/dashboard/admin", icon: "admin" }} />
          </div>
        </section>
      ) : null}
    </div>
  );
}
