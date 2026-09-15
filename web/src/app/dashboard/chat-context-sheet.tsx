"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import type { WorkspacesResource } from "@/lib/contracts/resources";
import { AppIcon } from "./app-icon";
import { useTranslations } from "@/lib/i18n/provider";
import styles from "./chat-mobile.module.css";

type WorkspaceItem = WorkspacesResource["items"][number];
type ChatMode = "ask" | "plan" | "agent";

type ChatContextSheetProps = {
  open: boolean;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  imageDisabled: boolean;
  workspaceItems: WorkspaceItem[];
  selectedWorkspaceKey: string;
  mode: ChatMode;
  models: string[];
  selectedModel: string;
  goal: string;
  modelLabel: (model: string) => string;
  workspaceKey: (workspace: WorkspaceItem) => string;
  workspaceStatusLabel: (workspace: WorkspaceItem) => string;
  onClose: () => void;
  onAttach: () => void;
  onWorkspaceChange: (value: string) => void;
  onModeChange: (value: ChatMode) => void;
  onModelChange: (value: string) => void;
  onGoalChange: (value: string) => void;
};

export function ChatContextSheet({
  open,
  triggerRef,
  imageDisabled,
  workspaceItems,
  selectedWorkspaceKey,
  mode,
  models,
  selectedModel,
  goal,
  modelLabel,
  workspaceKey,
  workspaceStatusLabel,
  onClose,
  onAttach,
  onWorkspaceChange,
  onModeChange,
  onModelChange,
  onGoalChange,
}: ChatContextSheetProps) {
  const { locale, t } = useTranslations();
  const closeRef = useRef<HTMLButtonElement>(null);
  const [modelSearch, setModelSearch] = useState("");
  const visibleModels = useMemo(() => {
    const query = modelSearch.trim().toLocaleLowerCase(locale);
    return query ? models.filter((model) => `${modelLabel(model)} ${model}`.toLocaleLowerCase(locale).includes(query)) : models;
  }, [locale, modelLabel, modelSearch, models]);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      onClose();
      triggerRef.current?.focus();
    };
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", closeOnEscape);
    window.requestAnimationFrame(() => closeRef.current?.focus());
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [onClose, open, triggerRef]);

  if (!open) return null;

  const closeAndRestore = () => {
    onClose();
    window.requestAnimationFrame(() => triggerRef.current?.focus());
  };

  return (
    <div className={styles.sheetLayer}>
      <button className={styles.sheetBackdrop} type="button" onClick={closeAndRestore} aria-label={t("Close context options")} />
      <section className={styles.sheet} role="dialog" aria-modal="true" aria-labelledby="chat-context-title">
        <div className={styles.sheetGrabber} aria-hidden="true" />
        <header className={styles.sheetHead}>
          <div>
            <h2 id="chat-context-title">{t("Task context")}</h2>
            <p>{t("Images, projects, mode and model")}</p>
          </div>
          <button ref={closeRef} type="button" onClick={closeAndRestore} aria-label={t("Close context options")}><AppIcon name="close" size={18} /></button>
        </header>
        <div className={styles.sheetBody}>
          <button className={styles.attachAction} type="button" onClick={onAttach} disabled={imageDisabled}>
            <span><AppIcon name="image" size={20} /></span>
            <span><strong>{t("Add image")}</strong><small>{t("PNG, JPG, or an image from your library")}</small></span>
            <AppIcon name="chevron-right" size={17} />
          </button>

          <fieldset className={styles.projectField}>
            <legend>{t("Project")}</legend>
            <label className={selectedWorkspaceKey === "auto" ? styles.selectedCard : undefined}>
              <input type="radio" name="mobile-project" value="auto" checked={selectedWorkspaceKey === "auto"} onChange={(event) => onWorkspaceChange(event.target.value)} />
              <span><strong>{t("Auto")}</strong><small>{t("CodeLocal chooses the most relevant project automatically")}</small></span>
            </label>
            {workspaceItems.map((workspace) => {
              const key = workspaceKey(workspace);
              return <label className={selectedWorkspaceKey === key ? styles.selectedCard : undefined} key={key}>
                <input type="radio" name="mobile-project" value={key} checked={selectedWorkspaceKey === key} onChange={(event) => onWorkspaceChange(event.target.value)} />
                <span><strong>{workspace.workspaceName}</strong><small>{workspace.deviceName} · {workspaceStatusLabel(workspace)}</small></span>
                <i data-status={workspace.status} data-online={workspace.runtimeOnline} aria-hidden="true" />
              </label>;
            })}
            <Link className={styles.manageProjects} href="/dashboard/workspaces">{t("Manage projects")} <AppIcon name="chevron-right" size={16} /></Link>
          </fieldset>

          <fieldset className={styles.modeField}>
            <legend>{t("Mode")}</legend>
            <div>
              {(["ask", "plan", "agent"] as const).map((value) => (
                <button key={value} type="button" className={mode === value ? styles.modeActive : ""} aria-pressed={mode === value} onClick={() => onModeChange(value)}>
                  {value === "ask" ? "Ask" : value === "plan" ? "Plan" : "Agent"}
                </button>
              ))}
            </div>
          </fieldset>

          <fieldset className={styles.modelField}>
            <legend>{t("Model")}</legend>
            {models.length > 10 ? <label className={styles.modelSearch}><AppIcon name="search" size={17} /><input value={modelSearch} onChange={(event) => setModelSearch(event.target.value)} placeholder={t("Search models")} aria-label={t("Search models")} /></label> : null}
            <div className={styles.modelList}>
              {visibleModels.map((model) => <button className={selectedModel === model ? styles.selectedModel : ""} type="button" aria-pressed={selectedModel === model} onClick={() => onModelChange(model)} key={model}>
                <span>{modelLabel(model)}</span>{modelLabel(model) !== model ? <small>{model}</small> : null}
              </button>)}
              {!visibleModels.length ? <p>{t("No models found.")}</p> : null}
            </div>
          </fieldset>

          <label className={styles.goalField}>
            <span>{t("Goal")} <small>{t("optional")}</small></span>
            <textarea value={goal} onChange={(event) => onGoalChange(event.target.value)} placeholder={t("Desired task outcome")} maxLength={240} rows={2} />
          </label>
        </div>
        <footer className={styles.sheetFooter}>
          <button type="button" onClick={closeAndRestore}>{t("Done")}</button>
        </footer>
      </section>
    </div>
  );
}
