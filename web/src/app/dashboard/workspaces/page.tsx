import { LiveWorkspaces } from "./workspaces-live";
import styles from "../dashboard.module.css";

export default function WorkspacesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Projects</span>
          <h1>Projects</h1>
          <p>Quản lý project và mở Chat hoặc Brain đúng context.</p>
        </div>
      </header>
      <LiveWorkspaces />
    </section>
  );
}
