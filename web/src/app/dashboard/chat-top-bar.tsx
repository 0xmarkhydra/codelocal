"use client";

import { AppIcon } from "./app-icon";
import styles from "./chat-mobile.module.css";

type ChatTopBarProps = {
  drawerOpen: boolean;
  title: string;
  subtitle: string;
  disabled: boolean;
  menuRef: React.RefObject<HTMLButtonElement | null>;
  onOpenMenu: () => void;
  onNewThread: () => void;
};

export function ChatTopBar({ drawerOpen, title, subtitle, disabled, menuRef, onOpenMenu, onNewThread }: ChatTopBarProps) {
  return (
    <header className={styles.topBar}>
      <button ref={menuRef} className={styles.topBarAction} type="button" onClick={onOpenMenu} aria-label="Mở danh sách tác vụ" aria-expanded={drawerOpen}>
        <AppIcon name="menu" size={20} />
      </button>
      <div className={styles.threadContext}>
        <strong title={title}>{title}</strong>
        <span title={subtitle}>{subtitle}</span>
      </div>
      <button className={styles.topBarAction} type="button" onClick={onNewThread} disabled={disabled} aria-label="Tạo tác vụ mới">
        <AppIcon name="plus" size={21} />
      </button>
    </header>
  );
}
