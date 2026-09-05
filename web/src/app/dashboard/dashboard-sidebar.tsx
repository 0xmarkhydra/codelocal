"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import { AppIcon } from "./app-icon";
import { DashboardNav } from "./dashboard-nav";
import styles from "./dashboard.module.css";

export function DashboardSidebar() {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    document.documentElement.toggleAttribute("data-menu-open", open);
    return () => document.documentElement.removeAttribute("data-menu-open");
  }, [open]);

  return <>
    <button className={styles.mobileMenuTrigger} type="button" onClick={() => setOpen(true)} aria-label="Open navigation" aria-expanded={open}>
      <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
    </button>
    <button className={`${styles.mobileMenuBackdrop} ${open ? styles.mobileMenuBackdropOpen : ""}`} type="button" onClick={() => setOpen(false)} aria-label="Close navigation" />
    <aside className={`${styles.sidebar} ${open ? styles.sidebarOpen : ""}`}>
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
