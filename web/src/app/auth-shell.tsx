import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import styles from "./auth-surface.module.css";

type AuthShellProps = {
  eyebrow: string;
  title: string;
  description: string;
  panelTitle: string;
  panelSubtitle: string;
  children: ReactNode;
  highlights?: string[];
};

export function AuthShell({ eyebrow, description, panelTitle, panelSubtitle, children, highlights }: AuthShellProps) {
  const trustItems = highlights ?? [
    "Go owns identity and authorization",
    "Source stays on explicitly authorized machines",
    "AI clients receive only the workspace access you grant",
  ];

  return (
    <main className={styles.page}>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
          <span>CodeLocal</span>
        </Link>
        <nav className={styles.navLinks} aria-label="Public navigation">
          <Link href="/security">Security</Link>
          <Link href="/support">Support</Link>
          <Link href="/dashboard">Open app</Link>
        </nav>
      </header>

      <section className={styles.main}>
        <div className={styles.authStage}>
          <div className={styles.authIntro}>
            <span className={styles.eyebrow}>{eyebrow}</span>
            <p>{description}</p>
          </div>

          <section className={styles.panel} aria-label={panelTitle}>
            <span className={styles.accountLabel}>CodeLocal account</span>
            <h1>{panelTitle}</h1>
            <p className={styles.subtitle}>{panelSubtitle}</p>
            {children}
          </section>

          <div className={styles.trustLine} aria-label="CodeLocal trust boundaries">
            {trustItems.map((item) => <span key={item}>{item}</span>)}
          </div>
        </div>
      </section>

      <footer className={styles.footer}>
        <span>Private execution · durable project intelligence</span>
        <div className={styles.footerLinks}>
          <Link href="/privacy">Privacy</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/security">Security</Link>
        </div>
      </footer>
    </main>
  );
}
