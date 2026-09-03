import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import { ShotsHub } from "./shots-hub";
import styles from "./shots.module.css";

export default function ShotsPage() {
  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.hero}>
          <span className={styles.heroIcon} aria-hidden="true"><AppIcon name="image" size={20} /></span>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>Visual sharing</span>
            <h1>Shots</h1>
            <p>Upload a screenshot, get a clean public link, and revoke access whenever you want.</p>
          </div>
          <span className={styles.heroBadge}><i /> Link-controlled</span>
        </header>
        <ShotsHub />
      </div>
    </section>
  );
}
