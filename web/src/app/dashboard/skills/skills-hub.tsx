"use client";

import { useEffect, useMemo, useState } from "react";
import { AppIcon } from "../app-icon";
import styles from "./skills.module.css";

type SkillView = "for-you" | "explore" | "mine" | "built-in";

type SkillItem = {
  id: string;
  name: string;
  version: string;
  publisher: string;
  scope: "system" | "personal" | "community" | string;
  kind: "knowledge" | "workflow" | "runtime" | "hybrid" | string;
  tags?: string[];
  capabilities?: string[];
  quality: number;
  verified: boolean;
  license?: string;
};

type SkillsResource = {
  autoUse: boolean;
  items: SkillItem[];
};

type OverviewResponse = {
  skills?: SkillsResource;
};

const views: Array<{ id: SkillView; label: string }> = [
  { id: "for-you", label: "For You" },
  { id: "explore", label: "Explore" },
  { id: "mine", label: "My Skills" },
  { id: "built-in", label: "Built-in" },
];

function scopeLabel(scope: string) {
  switch (scope) {
    case "system": return "Built-in";
    case "personal": return "Personal";
    case "community": return "Community";
    default: return scope;
  }
}

function permissionLabel(skill: SkillItem) {
  const capabilities = skill.capabilities?.filter(Boolean) ?? [];
  if (skill.kind === "knowledge" && capabilities.length === 0) return "Knowledge only · no runtime permissions";
  if (capabilities.length === 0) return `${skill.kind} · authorization required at execution`;
  return `Requires ${capabilities.join(", ")}`;
}

function SkillCard({ skill }: { skill: SkillItem }) {
  const tags = (skill.tags ?? []).slice(0, 4);
  return (
    <article className={styles.skillCard}>
      <div className={styles.skillIcon} aria-hidden="true">
        <AppIcon name="skill" size={21} />
      </div>
      <div className={styles.skillBody}>
        <div className={styles.skillHeading}>
          <div>
            <h2>{skill.name}</h2>
            <p>{skill.publisher} · {scopeLabel(skill.scope)} · v{skill.version}</p>
          </div>
          <span className={styles.autoBadge}>{skill.verified ? "Verified · Auto-use" : "Eligible"}</span>
        </div>
        <p className={styles.description}>
          {tags.length ? tags.join(" · ") : "Reusable CodeLocal expertise selected when it materially improves the task."}
        </p>
        {tags.length > 0 && (
          <div className={styles.tags} aria-label={`${skill.name} capabilities`}>
            {tags.map((tag) => <span key={tag}>{tag}</span>)}
          </div>
        )}
        <div className={styles.permissionLine}>
          <AppIcon name="shield" size={15} />
          <span>{permissionLabel(skill)}</span>
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
  const [resource, setResource] = useState<SkillsResource>({ autoUse: true, items: [] });
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v1/dashboard/overview", { credentials: "include", cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error(String(response.status));
        return response.json() as Promise<OverviewResponse>;
      })
      .then((data) => {
        if (cancelled) return;
        const skills = data.skills;
        if (!skills || !Array.isArray(skills.items)) throw new Error("skills catalog missing");
        setResource({ autoUse: skills.autoUse !== false, items: skills.items });
        setLoadFailed(false);
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  const visibleSkills = useMemo(() => {
    switch (view) {
      case "built-in":
        return resource.items.filter((skill) => skill.scope === "system");
      case "explore":
        return resource.items.filter((skill) => skill.scope === "community");
      case "mine":
        return resource.items.filter((skill) => skill.scope === "personal");
      default:
        return resource.items.filter((skill) => skill.verified);
    }
  }, [resource.items, view]);

  const emptyCopy = view === "explore"
    ? { title: "Community skills will appear here", copy: "Explore is the marketplace surface. Auto-routing remains the default, so normal users never need to browse it to benefit from Skills." }
    : view === "mine"
      ? { title: "Your reusable skills", copy: "Personal skills you create will appear here without mixing project-specific knowledge into Project Brain." }
      : { title: "No skills available yet", copy: "CodeLocal will surface reusable expertise here when the authoritative catalog contains a match." };

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
          {resource.autoUse ? "Auto-use on" : "Auto-use off"}
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
        <div className={styles.sectionIntro}>
          <h2>{view === "for-you" ? "Ready when relevant" : view === "built-in" ? "Built-in intelligence" : view === "explore" ? "Community market" : "Your reusable skills"}</h2>
          <p>{view === "for-you" ? "No install step. Safe knowledge skills can be selected directly from chat." : "This view is driven by the same authoritative catalog used by CodeLocal routing."}</p>
        </div>

        {loading && <EmptyView title="Loading Skills" copy="Reading the current CodeLocal catalog…" />}
        {!loading && loadFailed && <EmptyView title="Skills unavailable" copy="The catalog could not be loaded. Chat continues with the backend routing defaults." />}
        {!loading && !loadFailed && visibleSkills.map((skill) => <SkillCard key={`${skill.id}@${skill.version}`} skill={skill} />)}
        {!loading && !loadFailed && visibleSkills.length === 0 && <EmptyView title={emptyCopy.title} copy={emptyCopy.copy} />}
      </div>
    </section>
  );
}
