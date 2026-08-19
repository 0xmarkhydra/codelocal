import Image from "next/image";
import Link from "next/link";
import { DashboardNav } from "./dashboard-nav";
import styles from "./dashboard.module.css";

export default function DashboardLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <main className={styles.shell}>
      <aside className={styles.sidebar}>
        <Link className={styles.brand} href="/">
          <span className={styles.brandMark} aria-hidden="true">
            <Image src="/codelocal-icon.png" alt="" width={34} height={34} priority />
          </span>
          <span>CodeLocal</span>
        </Link>

        <div className={styles.navLabel}>Control plane</div>
        <DashboardNav />

        <div className={styles.sidebarNotice}>
          <span>Privacy boundary</span>
          <p>Source code, credentials and local project roots stay outside the browser presentation contract.</p>
        </div>
      </aside>
      {children}
    </main>
  );
}
