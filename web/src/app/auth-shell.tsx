import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import styles from "./auth-surface.module.css";
import { LanguageSelect } from "@/lib/i18n/provider";
import { getTranslations } from "@/lib/i18n/server";

type AuthShellProps = {
  eyebrow: string;
  title: string;
  description: string;
  panelTitle: string;
  panelSubtitle: string;
  children: ReactNode;
  highlights?: string[];
};

export async function AuthShell({ panelTitle, panelSubtitle, children }: AuthShellProps) {
  const t = await getTranslations();
  return (
    <main className={styles.page}>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label={t("CodeLocal home")}>
          <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
          <span>CodeLocal</span>
        </Link>
        <nav className={styles.navLinks} aria-label={t("Public navigation")}>
          <Link href="/security">{t("Security")}</Link>
          <Link href="/support">{t("Support")}</Link>
          <Link href="/dashboard">{t("Open app")}</Link>
          <LanguageSelect />
        </nav>
      </header>

      <section className={styles.main}>
        <div className={styles.authStage}>
          <section className={styles.panel} aria-label={panelTitle}>
            <span className={styles.accountLabel}>{t("CodeLocal account")}</span>
            <h1>{panelTitle}</h1>
            <p className={styles.subtitle}>{panelSubtitle}</p>
            {children}
          </section>
        </div>
      </section>

      <footer className={styles.footer}>
        <span>CodeLocal</span>
        <div className={styles.footerLinks}>
          <Link href="/privacy">{t("Privacy")}</Link>
          <Link href="/terms">{t("Terms")}</Link>
          <Link href="/security">{t("Security")}</Link>
        </div>
      </footer>
    </main>
  );
}
