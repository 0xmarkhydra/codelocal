import Link from "next/link";
import { AppIcon } from "./app-icon";
import { LiveOverview } from "./overview-live";
import styles from "./overview.module.css";

export const metadata = { title: "Tổng quan" };

export default function DashboardPage() {
  return (
    <section className={styles.page} lang="vi">
      <div className={styles.topbar}>
        <span>
          Workspace <span aria-hidden="true">/</span> <strong>Tổng quan</strong>
        </span>
        <Link href="/dashboard/connect">
          <AppIcon name="plus" size={15} /> Kết nối AI
        </Link>
      </div>
      <div className={styles.content}>
        <header className={styles.heading}>
          <div>
            <span className={styles.eyebrow}>CODELOCAL / WORKSPACE</span>
            <h1>Tổng quan</h1>
            <p>Dự án, thiết bị và hoạt động trong tài khoản của bạn.</p>
          </div>
          <Link className={styles.outlineButton} href="/dashboard/workspaces">
            <AppIcon name="folder" size={15} /> Xem dự án
          </Link>
        </header>
        <LiveOverview />
        <Link className={styles.brainLink} href="/dashboard/knowledge">
          <AppIcon name="brain" size={24} />
          <div><strong>Project Brain</strong><span>Tri thức, ngữ cảnh và kinh nghiệm của dự án.</span></div>
          <AppIcon name="chevron-right" size={18} />
        </Link>
        <section className={styles.desktopBanner}>
          <span className={styles.desktopIcon}>
            <AppIcon name="device" size={25} />
          </span>
          <div>
            <strong>CodeLocal Desktop</strong>
            <p>Đang phát triển cho macOS, Windows và Linux.</p>
          </div>
          <Link href="/#desktop">
            Lộ trình <AppIcon name="external" size={15} />
          </Link>
        </section>
        <footer className={styles.footer}>
          <span>
            <AppIcon name="shield" size={13} /> Thực thi cục bộ. Quyền truy cập
            theo workspace.
          </span>
          <Link href="/support">
            Cần hỗ trợ? <span aria-hidden="true">↗</span>
          </Link>
        </footer>
      </div>
    </section>
  );
}
