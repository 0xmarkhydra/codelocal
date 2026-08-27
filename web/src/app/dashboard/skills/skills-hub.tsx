"use client";

import { useState } from "react";
import { AppIcon } from "../app-icon";
import styles from "./skills.module.css";

type SkillView = "for-you" | "explore" | "mine" | "built-in";

const views: Array<{ id: SkillView; label: string }> = [
  { id: "for-you", label: "For You" },
  { id: "explore", label: "Explore" },
  { id: "mine", label: "My Skills" },
  { id: "built-in", label: "Built-in" },
];

function BuiltinSkillCard() {
  return (
    <article className={styles.skillCard}>
      <div className={styles.skillIcon} aria-hidden="true">
        <AppIcon name="skill" size={21} />
      </div>
      <div className={styles.skillBody}>
        <div className={styles.skillHeading}>
          <div>
            <h2>UI/UX Pro</h2>
            <p>CodeLocal · Built-in</p>
          </div>
          <span className={styles.autoBadge}>Auto-use</span>
        </div>
        <p className={styles.description}>
          Design, accessibility, typography, responsive layout and visual review knowledge. CodeLocal can select it automatically when it materially improves a task.
        </p>
        <div className={styles.tags} aria-label="UI/UX Pro capabilities">
          <span>UI</span>
          <span>UX</span>
          <span>Frontend</span>
          <span>Accessibility</span>
        </div>
        <div className={styles.permissionLine}>
          <AppIcon name="shield" size={15} />
          <span>Knowledge only · no runtime permissions</span>
        </div>
      </div>
    </article>
  );
}

function EmptyView({ title, copy }: { title: string; copy: string }) {
  return (
    <div className={styles.emptyState}>
      <AppIcon name="skill" size={22} />
      <strong>{title}</strong>
      <span>{copy}</span>
    </div>
  );
}

export function SkillsHub() {
  const [view, setView] = useState<SkillView>("for-you");

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>CodeLocal Intelligence</p>
          <h1>Skills</h1>
          <p className={styles.lede}>Chat normally. CodeLocal finds and combines useful expertise automatically.</p>
        </div>
        <div className={styles.status}>
          <span className={styles.statusDot} aria-hidden="true" />
          Auto-use on
        </div>
      </header>

      <nav className={styles.tabs} aria-label="Skill views">
        {views.map((item) => (
          <button
            aria-current={view === item.id ? "page" : undefined}
            className={view === item.id ? styles.activeTab : undefined}
            key={item.id}
            onClick={() => setView(item.id)}
            type="button"
          >
            {item.label}
          </button>
        ))}
      </nav>

      <div className={styles.content}>
        {view === "for-you" && (
          <>
            <div className={styles.sectionIntro}>
              <h2>Ready when relevant</h2>
              <p>No install step. Safe knowledge skills can be selected directly from chat.</p>
            </div>
            <BuiltinSkillCard />
          </>
        )}

        {view === "built-in" && (
          <>
            <div className={styles.sectionIntro}>
              <h2>Built-in intelligence</h2>
              <p>Maintained centrally by CodeLocal and available to every user.</p>
            </div>
            <BuiltinSkillCard />
          </>
        )}

        {view === "explore" && (
          <EmptyView
            title="Community skills will appear here"
            copy="Explore is the marketplace surface. Auto-routing remains the default, so normal users never need to browse it to benefit from Skills."
          />
        )}

        {view === "mine" && (
          <EmptyView
            title="Your published skills"
            copy="Private and community contributions will live here without mixing project-specific knowledge into Project Brain."
          />
        )}
      </div>
    </section>
  );
}
