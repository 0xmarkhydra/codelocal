import Image from "next/image";
import { AppIcon } from "../dashboard/app-icon";
import styles from "./intelligence-core.module.css";

/** An abstract product illustration, never a visualization of runtime activity. */
export function IntelligenceCore({ compact = false }: { compact?: boolean }) {
  return (
    <div
      className={`${styles.scene} ${compact ? styles.compact : ""}`}
      aria-hidden="true"
    >
      <div className={styles.halo} />
      <div className={styles.orbit} />
      <div className={styles.orbitTwo} />
      <div className={styles.structure}>
        <div className={`${styles.plate} ${styles.base}`} />
        <div className={`${styles.plate} ${styles.middle}`} />
        <div className={`${styles.plate} ${styles.top}`}>
          <Image
            className={styles.logo}
            src="/codelocal-icon.png"
            alt=""
            width={240}
            height={240}
            sizes="240px"
          />
        </div>
        <div className={`${styles.satellite} ${styles.cyan}`}>
          <AppIcon name="brain" size={34} />
        </div>
        <div className={`${styles.satellite} ${styles.orange}`}>
          <AppIcon name="terminal" size={32} />
        </div>
        <div className={`${styles.satellite} ${styles.pink}`}>
          <AppIcon name="skill" size={34} />
        </div>
      </div>
      {!compact && (
        <>
          <span className={`${styles.label} ${styles.context}`}>
            01 / PROJECT BRAIN
          </span>
          <span className={`${styles.label} ${styles.local}`}>
            02 / LOCAL RUNTIME
          </span>
          <span className={`${styles.label} ${styles.skills}`}>
            03 / VERIFIED SKILLS
          </span>
        </>
      )}
    </div>
  );
}
