"use client";

import { AppIcon } from "./app-icon";
import { useTranslations } from "@/lib/i18n/provider";
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
  const { t } = useTranslations();
  return (
    <header className={styles.topBar}>
      <button ref={menuRef} className={styles.topBarAction} type="button" onClick={onOpenMenu} aria-label={t("Open task list")} aria-expanded={drawerOpen}>
        <AppIcon name="menu" size={20} />
      </button>
      <div className={styles.threadContext}>
        <strong title={title}>{title}</strong>
        <span title={subtitle}>{subtitle}</span>
      </div>
      <button className={styles.topBarAction} type="button" onClick={onNewThread} disabled={disabled} aria-label={t("Create new task")}>
        <AppIcon name="plus" size={21} />
      </button>
    </header>
  );
}
