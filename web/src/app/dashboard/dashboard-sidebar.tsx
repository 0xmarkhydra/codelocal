"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import { AppIcon } from "./app-icon";
import { DashboardNav } from "./dashboard-nav";
import styles from "./dashboard.module.css";

export function DashboardSidebar({ title }: { title: string }) {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    document.documentElement.toggleAttribute("data-menu-open", open);
    return () => document.documentElement.removeAttribute("data-menu-open");
  }, [open]);

  return <>
    <header className={styles.mobileTopBar}>
      <button className={styles.mobileMenuTrigger} type="button" onClick={() => setOpen(true)} aria-label="Open navigation" aria-expanded={open}>
        <AppIcon name="menu" size={22} />
      </button>
      <strong>{title}</strong>
      <span className={styles.mobileTopBarRight} aria-hidden="true" />
    </header>
    {open ? <button className={`${styles.mobileMenuBackdrop} ${styles.mobileMenuBackdropOpen}`} type="button" onClick={() => setOpen(false)} aria-label="Close navigation" /> : null}
    <aside className={`${styles.sidebar} ${open ? styles.sidebarOpen : ""}`} aria-hidden={!open || undefined} aria-modal={open || undefined} role={open ? "dialog" : undefined}>
      <div className={styles.brandRow}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <span className={styles.brandMark} aria-hidden="true"><Image src="/codelocal-icon.png" alt="" width={28} height={28} priority /></span>
          <span>CodeLocal</span>
        </Link>
        <div className={styles.brandActions}>
          <Link className={styles.downloadButton} href="/#how-it-works" aria-label="Install CodeLocal" title="Install CodeLocal"><AppIcon name="download" size={16} /></Link>
          <button className={styles.mobileMenuClose} type="button" onClick={() => setOpen(false)} aria-label="Close navigation">×</button>
        </div>
      </div>
      <DashboardNav onNavigate={() => setOpen(false)} />
    </aside>
  </>;
}
