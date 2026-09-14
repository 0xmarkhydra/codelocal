import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import styles from "./forums.module.css";

export default function ForumsLayout({ children }: { children: ReactNode }) {
  return (
    <div className={styles.shell}>
      <header className={styles.topbar}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <Image src="/codelocal-icon.png" alt="" width={30} height={30} />
          <span>CodeLocal.</span>
        </Link>
        <nav className={styles.nav} aria-label="Community navigation">
          <Link href="/">Product</Link>
          <Link href="/forums" aria-current="page">Forums</Link>
          <Link href="/blogs">Journal</Link>
          <Link href="/security">Security</Link>
        </nav>
        <Link className={styles.dashboardLink} href="/dashboard/forums">Open dashboard</Link>
      </header>
      {children}
      <footer className={styles.footer}>
        <span>CodeLocal Community</span>
        <nav><Link href="/forums">Forums</Link><Link href="/support">Support</Link><Link href="/privacy">Privacy</Link><Link href="/terms">Terms</Link></nav>
      </footer>
    </div>
  );
}
