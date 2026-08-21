import Image from "next/image";
import Link from "next/link";
import { DashboardNav } from "./dashboard-nav";
import styles from "./dashboard.module.css";

export default function DashboardLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <main className={styles.shell}>
      <aside className={styles.sidebar}>
        <div className={styles.brandRow}>
          <Link className={styles.brand} href="/" aria-label="CodeLocal home">
            <span className={styles.brandMark} aria-hidden="true">
              <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
            </span>
            <span>CodeLocal</span>
          </Link>
          <Link className={styles.downloadButton} href="/#how-it-works" aria-label="Install CodeLocal" title="Install CodeLocal">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 3v11m0 0 4-4m-4 4-4-4M5 17v2h14v-2" />
            </svg>
          </Link>
        </div>

        <DashboardNav />
      </aside>
      {children}
    </main>
  );
}
