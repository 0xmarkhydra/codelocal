import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import { RuntimeSettingsLive } from "./runtime-settings-live";
import { SettingsHub } from "./settings-hub";
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
          <span className={styles.scopeHint}>Project · Device · Account</span>
        </header>
        <SettingsHub />
        <div className={styles.runtimeHeading} id="runtime">
          <span>Advanced</span>
          <h2>Runtime settings</h2>
          <p>Cấu hình runtime theo global, device hoặc workspace.</p>
        </div>
        <RuntimeSettingsLive />
      </div>
    </section>
  );
}
