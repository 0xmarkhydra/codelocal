import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import { RuntimeSettingsLive } from "./runtime-settings-live";
import styles from "./runtime-settings.module.css";

export default function SettingsPage() {
  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.hero}>
          <span className={styles.heroIcon} aria-hidden="true"><AppIcon name="settings" size={20} /></span>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>Control settings</span>
            <h1>Settings</h1>
            <p>Config, secrets và system capabilities cho CodeLocal Runtime.</p>
          </div>
          <span className={styles.scopeHint}>Global · Device · Workspace</span>
        </header>
        <RuntimeSettingsLive />
      </div>
    </section>
  );
}
