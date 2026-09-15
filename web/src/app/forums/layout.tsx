import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import { AppIcon } from "@/app/dashboard/app-icon";
import { getTranslations } from "@/lib/i18n/server";
import styles from "./forums.module.css";

export default async function ForumsLayout({ children }: { children: ReactNode }) {
  const t = await getTranslations();
  return (
    <div className={styles.shell}>
      <header className={styles.navbar}>
        <Link className={styles.brand} href="/" aria-label={t("CodeLocal home")}>
          <Image src="/codelocal-icon.png" alt="" width={32} height={32} />
          CodeLocal
        </Link>
        <nav className={styles.navLinks} aria-label={t("Primary navigation")}>
          <Link href="/">{t("Product")}</Link>
          <Link href="/forums" aria-current="page">{t("Forums")}</Link>
          <Link href="/security">{t("Security")}</Link>
          <Link href="/blogs">{t("Journal")}</Link>
        </nav>
        <Link className={styles.navButton} href="/dashboard/forums">
          {t("Open dashboard")} <AppIcon name="external" size={15} />
        </Link>
      </header>
      {children}
      <footer className={styles.footer}>
        <Link className={styles.brand} href="/">
          <Image src="/codelocal-icon.png" alt="" width={28} height={28} />
          CodeLocal
        </Link>
        <nav aria-label={t("Footer navigation")}>
          <Link href="/forums">{t("Forums")}</Link><Link href="/blogs">{t("Journal")}</Link><Link href="/security">{t("Security")}</Link><Link href="/privacy">{t("Privacy")}</Link><Link href="/terms">{t("Terms")}</Link><Link href="/support">{t("Contact")}</Link><Link href="/login">{t("Sign in")}</Link>
        </nav>
      </footer>
    </div>
  );
}
