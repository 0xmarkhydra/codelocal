"use client";

import Image from "next/image";
import Link from "next/link";
import { useLayoutEffect, useRef, useState } from "react";
import { AppIcon } from "./app-icon";
import { DashboardNav } from "./dashboard-nav";
import styles from "./dashboard-chrome.module.css";

export function DashboardSidebar() {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const sidebar = useRef<HTMLElement>(null);
  const close = () => {
    setOpen(false);
    trigger.current?.focus();
  };

  useLayoutEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const focusable = () =>
      Array.from(
        sidebar.current?.querySelectorAll<HTMLElement>(
          "a[href], button, summary",
        ) ?? [],
      ).filter((element) => {
        const details = element.closest("details");
        return (
          element.getClientRects().length > 0 &&
          (!details || details.open || element.tagName === "SUMMARY")
        );
      });
    focusable()[0]?.focus();
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        trigger.current?.focus();
      }
      if (event.key !== "Tab") return;
      const items = focusable();
      const first = items[0];
      const last = items.at(-1);
      if (!sidebar.current?.contains(document.activeElement)) {
        event.preventDefault();
        (event.shiftKey ? last : first)?.focus();
        return;
      }
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      }
      if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    const desktop = window.matchMedia("(min-width: 821px)");
    const handleResize = () => {
      if (desktop.matches) setOpen(false);
    };
    desktop.addEventListener("change", handleResize);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", handleKey);
      desktop.removeEventListener("change", handleResize);
    };
  }, [open]);

  return (
    <>
      <div className={styles.mobileBar}>
        <Link className={styles.brand} href="/">
          <Image src="/codelocal-icon.png" alt="" width={27} height={27} />
          CodeLocal.
        </Link>
        <button
          ref={trigger}
          type="button"
          onClick={() => setOpen(true)}
          aria-label="Mở điều hướng"
          aria-controls="dashboard-sidebar"
          aria-expanded={open}
        >
          <AppIcon name="menu" size={21} />
        </button>
      </div>
      <button
        className={`${styles.backdrop} ${open ? styles.backdropOpen : ""}`}
        type="button"
        onClick={close}
        aria-label="Đóng điều hướng"
        tabIndex={-1}
      />
      <aside
        ref={sidebar}
        id="dashboard-sidebar"
        className={`${styles.sidebar} ${open ? styles.sidebarOpen : ""}`}
        role={open ? "dialog" : undefined}
        aria-modal={open || undefined}
        aria-label="Điều hướng dashboard"
      >
        <div className={styles.brandRow}>
          <Link className={styles.brand} href="/" aria-label="CodeLocal home">
            <Image
              src="/codelocal-icon.png"
              alt=""
              width={32}
              height={32}
              priority
            />
            <span>
              CodeLocal<span className={styles.brandDot}>.</span>
            </span>
          </Link>
          <button
            className={styles.closeButton}
            type="button"
            onClick={close}
            aria-label="Đóng điều hướng"
          >
            <AppIcon name="close" />
          </button>
        </div>
        <div className={styles.workspaceBadge}>
          <span className={styles.workspaceGlyph}>
            <AppIcon name="module" size={18} />
          </span>
          <div>
            <strong>Không gian làm việc</strong>
            <span>CodeLocal Cloud</span>
          </div>
        </div>
        <DashboardNav onNavigate={() => setOpen(false)} />
      </aside>
    </>
  );
}
