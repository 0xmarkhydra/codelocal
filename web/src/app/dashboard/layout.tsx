import Image from "next/image";
import Link from "next/link";
import { AppIcon } from "./app-icon";
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
            <AppIcon name="download" size={16} />
          </Link>
        </div>

        <DashboardNav />
      </aside>
      {children}
    </main>
  );
}
