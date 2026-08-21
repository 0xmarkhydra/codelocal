import { LiveWorkspaces } from "./workspaces-live";
import styles from "../dashboard.module.css";

export default function WorkspacesPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}><div><h1>Workspaces</h1></div></header>
      <LiveWorkspaces />
    </section>
  );
}
