import { RuntimeSettingsLive } from "./runtime-settings-live";
import styles from "../dashboard.module.css";

export default function SettingsPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Runtime</span>
          <h1>Settings</h1>
          <p>Config, secrets và capabilities dùng chung cho CodeLocal Runtime.</p>
        </div>
      </header>
      <RuntimeSettingsLive />
    </section>
  );
}
