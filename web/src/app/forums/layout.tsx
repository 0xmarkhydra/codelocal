import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";
import { AppIcon } from "@/app/dashboard/app-icon";
import styles from "./forums.module.css";

export default function ForumsLayout({ children }: { children: ReactNode }) {
  return (
    <div className={styles.shell}>
      <header className={styles.navbar}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal.Cloud home">
          <Image src="/codelocal-icon.png" alt="" width={32} height={32} />
          CodeLocal<span className={styles.brandDot}>.</span>Cloud
        </Link>
        <nav className={styles.navLinks} aria-label="Primary navigation">
          <Link href="/">Product</Link>
          <Link href="/forums" aria-current="page">Forums</Link>
          <Link href="/security">Security</Link>
          <Link href="/blogs">Journal</Link>
        </nav>
        <Link className={styles.navButton} href="/dashboard/forums">
          Open dashboard <AppIcon name="external" size={15} />
        </Link>
      </header>
      {children}
      <footer className={styles.footer}>
        <Link className={styles.brand} href="/">
          <Image src="/codelocal-icon.png" alt="" width={28} height={28} />
          CodeLocal<span className={styles.brandDot}>.</span>Cloud
        </Link>
        <nav aria-label="Footer navigation">
          <Link href="/forums">Forums</Link><Link href="/blogs">Journal</Link><Link href="/security">Security</Link><Link href="/privacy">Privacy</Link><Link href="/terms">Terms</Link><Link href="/support">Contact</Link><Link href="/login">Sign in</Link>
        </nav>
      </footer>
    </div>
  );
}
