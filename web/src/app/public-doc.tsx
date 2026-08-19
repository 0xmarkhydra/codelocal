import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import styles from "./public-doc.module.css";

type Section = { title: string; body: ReactNode };
type Props = { title: string; eyebrow: string; summary: string; sections: Section[] };

export function PublicDoc({ title, eyebrow, summary, sections }: Props) {
  return (
    <main className={styles.page}>
      <nav className={styles.nav}>
        <Link className={styles.brand} href="/">
          <Image src="/codelocal-icon.png" alt="" width={34} height={34} />
          <span>CodeLocal</span>
        </Link>
        <div className={styles.navLinks}>
          <Link href="/privacy">Privacy</Link>
          <Link href="/security">Security</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/support">Support</Link>
        </div>
      </nav>
      <section className={styles.hero}>
        <span className={styles.eyebrow}>{eyebrow}</span>
        <h1>{title}</h1>
        <p>{summary}</p>
        <div className={styles.meta}>Publisher: CodeLocal · Last updated: August 19, 2026</div>
      </section>
      <section className={styles.content}>
        {sections.map((section) => <article className={styles.card} key={section.title}><h2>{section.title}</h2>{section.body}</article>)}
      </section>
      <footer className={styles.footer}>
        <span>© CodeLocal</span>
        <div className={styles.footerLinks}><Link href="/">Home</Link><Link href="/support">Support</Link></div>
      </footer>
    </main>
  );
}
