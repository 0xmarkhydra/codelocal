import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import { getLocale, getTranslations } from "@/lib/i18n/server";
import { LanguageSelect } from "@/lib/i18n/provider";
import styles from "./public-doc.module.css";

type Section = { title: string; body: ReactNode };
type Props = { title: string; eyebrow: string; summary: string; sections: Section[] };

export async function PublicDoc({ title, eyebrow, summary, sections }: Props) {
  const t = await getTranslations();
  const locale = await getLocale();
  const updatedAt = new Intl.DateTimeFormat(locale, { year: "numeric", month: "long", day: "numeric", timeZone: "UTC" }).format(new Date("2026-08-19T00:00:00Z"));
  return (
    <main className={styles.page}>
      <nav className={styles.nav} aria-label={t("Legal and support navigation")}>
        <Link className={styles.brand} href="/" aria-label={t("CodeLocal.Cloud home")}>
          <Image src="/codelocal-icon.png" alt="" width={34} height={34} />
          <span>CodeLocal.Cloud</span>
        </Link>
        <div className={styles.navLinks}>
          <Link href="/privacy">{t("Privacy")}</Link>
          <Link href="/security">{t("Security")}</Link>
          <Link href="/terms">{t("Terms")}</Link>
          <Link href="/support">{t("Support")}</Link>
        </div>
        <LanguageSelect />
      </nav>
      <section className={styles.hero}>
        <span className={styles.eyebrow}>{eyebrow}</span>
        <h1>{title}</h1>
        <p>{summary}</p>
        <div className={styles.meta}>{t("Publisher: CodeLocal.Cloud")} · <time dateTime="2026-08-19">{t("Last updated: {date}", { date: updatedAt })}</time></div>
      </section>
      <section className={styles.content}>
        {sections.map((section) => <article className={styles.card} key={section.title}><h2>{section.title}</h2>{section.body}</article>)}
      </section>
      <footer className={styles.footer}>
        <span>© CodeLocal.Cloud</span>
        <div className={styles.footerLinks}><Link href="/">{t("Home")}</Link><Link href="/support">{t("Support")}</Link></div>
      </footer>
    </main>
  );
}
