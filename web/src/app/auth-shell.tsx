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

export function AuthShell({ eyebrow, title, description, panelTitle, panelSubtitle, children, highlights }: AuthShellProps) {
  return (
    <main className={styles.page}>
      <header className={styles.nav}>
        <Link className={styles.brand} href="/">
          <Image src="/codelocal-icon.png" alt="" width={34} height={34} priority />
          <span>CodeLocal</span>
        </Link>
        <nav className={styles.navLinks} aria-label="Public navigation">
          <Link href="/security">Security</Link>
          <Link href="/support">Support</Link>
          <Link href="/dashboard">Dashboard</Link>
        </nav>
      </header>

      <section className={styles.main}>
        <div className={styles.frame}>
          <section className={styles.context}>
            <div>
              <span className={styles.eyebrow}>{eyebrow}</span>
              <h1>{title}</h1>
              <p>{description}</p>
            </div>
            <div className={styles.contextList}>
              {(highlights ?? [
                "Go remains the identity and authorization authority",
                "Local source stays on explicitly authorized machines",
                "MCP clients receive only the workspace access you grant",
              ]).map((item) => <div className={styles.contextItem} key={item}>{item}</div>)}
            </div>
          </section>

          <section className={styles.panel}>
            <span className={styles.eyebrow}>CodeLocal account</span>
            <h2>{panelTitle}</h2>
            <p className={styles.subtitle}>{panelSubtitle}</p>
            {children}
          </section>
        </div>
      </section>

      <footer className={styles.footer}>
        <span>CodeLocal · controlled local execution + durable project intelligence</span>
        <div className={styles.footerLinks}>
          <Link href="/privacy">Privacy</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/security">Security</Link>
        </div>
      </footer>
    </main>
  );
}
