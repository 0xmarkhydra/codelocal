import Image from "next/image";
import Link from "next/link";
import { DashboardNav } from "./dashboard-nav";
import styles from "./dashboard.module.css";

export default function DashboardLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <main className={styles.shell}>
      <aside className={styles.sidebar}>
        <Link className={styles.brand} href="/" aria-label="CodeLocal home">
          <span className={styles.brandMark} aria-hidden="true">
            <Image src="/codelocal-icon.png" alt="" width={28} height={28} priority />
          </span>
          <span>CodeLocal</span>
        </Link>

        <DashboardNav />
      </aside>
      {children}
    </main>
  );
}