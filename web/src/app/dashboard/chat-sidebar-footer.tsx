"use client";

import Image from "next/image";
import Link from "next/link";
import type { RefObject } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { AppIcon } from "./app-icon";
import { useDashboardResource } from "./use-dashboard-resource";
import controls from "./dashboard-controls.module.css";
import styles from "./chat-sidebar-footer.module.css";
import chatStyles from "./dashboard-chat.module.css";

export function ChatSidebarBrand({ closeRef, onClose }: { closeRef: RefObject<HTMLButtonElement | null>; onClose: () => void }) {
  return (
    <div className={chatStyles.unifiedSidebarBrand}>
      <Link className={chatStyles.unifiedBrandLink} href="/" aria-label="CodeLocal home">
        <Image src="/codelocal-icon.png" alt="" width={25} height={25} priority />
        <span>CodeLocal</span>
      </Link>
      <button ref={closeRef} className={chatStyles.unifiedSidebarClose} type="button" onClick={onClose} aria-label="Đóng menu">
        <AppIcon name="close" size={17} />
      </button>
    </div>
  );
}

export function ChatSidebarFooter({ onNavigate }: { onNavigate: () => void }) {
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;

  return (
    <footer className={styles.sidebarFooter}>
      <nav className={styles.sidebarFooterLinks} aria-label="Dashboard shortcuts">
        <Link href="/dashboard/workspaces" onClick={onNavigate}><AppIcon name="folder" size={17} /><span>All Projects</span></Link>
        <Link href="/dashboard/devices" onClick={onNavigate}><AppIcon name="device" size={17} /><span>Devices</span></Link>
        <Link href="/dashboard/settings" onClick={onNavigate}><AppIcon name="settings" size={17} /><span>Settings</span></Link>
      </nav>
      {readyAccount ? (
        <details className={`${controls.sidebarAccount} ${styles.sidebarAccount}`}>
          <summary className={controls.sidebarAccountSummary}>
            <span className={controls.sidebarAvatar} aria-hidden="true">{readyAccount.email.slice(0, 1).toUpperCase()}</span>
            <span className={controls.sidebarAccountCopy}><strong title={readyAccount.email}>{readyAccount.email}</strong><span>{readyAccount.isAdmin ? "Admin" : "Account"}</span></span>
            <AppIcon className={controls.sidebarChevron} name="chevron-down" size={14} />
          </summary>
          <div className={controls.sidebarAccountMenu}>
            <Link href="/dashboard/account" onClick={onNavigate}>Account</Link>
            <Link href="/dashboard/security" onClick={onNavigate}>Security</Link>
            <form method="post" action="/logout">
              <input type="hidden" name="csrf" value={readyAccount.csrf} />
              <input type="hidden" name="next" value="/dashboard" />
              <button className={controls.signOutButton} type="submit">Sign out</button>
            </form>
          </div>
        </details>
      ) : null}
    </footer>
  );
}
