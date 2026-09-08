"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isBlogSeriesResource } from "@/lib/contracts/blog";
import { MediaUploadError, privateMediaVariantURL, uploadMediaAsset } from "@/lib/media-upload";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey, MessageValues } from "@/lib/i18n/messages";
import { useUnsavedWarning } from "./use-unsaved-warning";
import { useDashboardResource } from "../use-dashboard-resource";
import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import styles from "./series-editor.module.css";

type SaveState = "idle" | "saving" | "saved" | "error";
type Notice = { key: MessageKey; values?: MessageValues } | null;

export function SeriesEditor({ seriesID }: { seriesID: string }) {
  const { t, message } = useTranslations();
  const router = useRouter();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const resource = useDashboardResource(`/api/v1/blog/series/${encodeURIComponent(seriesID)}`, isBlogSeriesResource);
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [status, setStatus] = useState<"active" | "complete" | "archived">("active");
  const [coverAssetID, setCoverAssetID] = useState("");
  const [hydratedID, setHydratedID] = useState("");
  const [dirty, setDirty] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [mediaBusy, setMediaBusy] = useState(false);
  const [actionError, setActionError] = useState<Notice>(null);
  const editVersion = useRef(0);
  const saveQueue = useRef<Promise<boolean>>(Promise.resolve(true));
  useUnsavedWarning(dirty || mediaBusy);

  const series = resource.state.kind === "ready" ? resource.state.value.series : undefined;

  useEffect(() => {
    if (!series || hydratedID === series.id) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- hydrate editable form state when the async series resource changes
    setTitle(series.title);
    setSlug(series.slug);
    setDescription(series.description);
    setStatus(series.status);
    setCoverAssetID(series.coverAssetId ?? "");
    setHydratedID(series.id);
    editVersion.current = 0;
    saveQueue.current = Promise.resolve(true);
    setDirty(false);
    setSaveState("idle");
  }, [hydratedID, series]);

  const payload = useMemo(() => ({ title, slug, description, status, coverAssetId: coverAssetID }), [coverAssetID, description, slug, status, title]);

  const save = useCallback(() => {
    if (!dirty) return Promise.resolve(true);
    if (!series || account.state.kind !== "ready") return Promise.resolve(false);
    const snapshot = payload;
    const version = editVersion.current;
    const csrf = account.state.value.csrf;

    const run = async () => {
      setSaveState("saving");
      setActionError(null);
      try {
        const response = await fetch(`/api/v1/blog/series/${encodeURIComponent(series.id)}`, {
          method: "PATCH",
          credentials: "same-origin",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
          },
          body: JSON.stringify(snapshot),
        });
        const body: unknown = await response.json().catch(() => null);
        if (!response.ok || !isBlogSeriesResource(body)) {
          if (editVersion.current === version) {
            setSaveState("error");
            setActionError(response.status === 409
              ? { key: "That series URL is already in use." }
              : { key: "Save failed ({status}).", values: { status: String(response.status) } });
          }
          return false;
        }
        if (editVersion.current === version) {
          setDirty(false);
          setSaveState("saved");
          return true;
        }
        setDirty(true);
        setSaveState("idle");
        return false;
      } catch {
        if (editVersion.current === version) {
          setSaveState("error");
          setActionError({ key: "Save failed." });
        }
        return false;
      }
    };

    const queued = saveQueue.current.then(run, run);
    saveQueue.current = queued.then(() => true, () => false);
    return queued;
  }, [account.state, dirty, payload, series]);

  useEffect(() => {
    if (!dirty || hydratedID !== series?.id || mediaBusy) return;
    const timer = window.setTimeout(() => { void save(); }, 1000);
    return () => window.clearTimeout(timer);
  }, [dirty, hydratedID, mediaBusy, save, series?.id]);

  function markDirty() {
    editVersion.current += 1;
    setDirty(true);
    setSaveState("idle");
  }

  function change(setter: (value: string) => void, value: string) {
    setter(value);
    markDirty();
  }

  async function uploadCover(file: File) {
    if (account.state.kind !== "ready") return;
    setMediaBusy(true);
    setActionError(null);
    try {
      const asset = await uploadMediaAsset(file, account.state.value.csrf);
      setCoverAssetID(asset.id);
      markDirty();
    } catch (error) {
      setActionError(error instanceof MediaUploadError
        ? { key: error.messageKey, values: error.values }
        : { key: "Image upload failed." });
    } finally {
      setMediaBusy(false);
    }
  }

  async function archiveSeries() {
    if (!series || account.state.kind !== "ready") return;
    if (!(await save())) return;
    setActionError(null);
    try {
      const response = await fetch(`/api/v1/blog/series/${encodeURIComponent(series.id)}`, {
        method: "DELETE",
        credentials: "same-origin",
        headers: { Accept: "application/json", "X-CSRF-Token": account.state.value.csrf },
      });
      if (!response.ok) {
        setActionError({ key: "Unable to archive series ({status}).", values: { status: String(response.status) } });
        return;
      }
      router.push("/dashboard/blogs");
      router.refresh();
    } catch {
      setActionError({ key: "Unable to archive series." });
    }
  }

  if (resource.state.kind === "loading") {
    return <section className={dashboard.content}><div className={styles.state}>{t("Loading series…")}</div></section>;
  }
  if (resource.state.kind !== "ready" || !series) {
    const notice = resource.state.kind === "error" ? message(resource.state.message) : t("This series is not available.");
    return <section className={dashboard.content}><div className={styles.state}>{notice}</div></section>;
  }

  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.topbar}>
          <Link href="/dashboard/blogs" aria-label={t("Back to blogs")} title={t("Back to blogs")}><AppIcon name="chevron-left" size={18} aria-hidden="true" /></Link>
          <span aria-live="polite">{mediaBusy ? t("Processing cover…") : saveState === "saving" ? t("Saving…") : saveState === "saved" ? t("Saved") : saveState === "error" ? t("Save failed") : dirty ? t("Unsaved") : ""}</span>
          <div>
            {series.status !== "archived" && <Link href={`/blogs/series/${series.slug}`}>{t("View series")}</Link>}
            <button type="button" onClick={() => void save()} disabled={!dirty || account.state.kind !== "ready" || mediaBusy || saveState === "saving"}>{t("Save")}</button>
          </div>
        </header>

        <main className={styles.form}>
          <div className={styles.heading}>
            <h1>{title || t("Untitled series")}</h1>
          </div>

          {coverAssetID && (
            <div className={styles.cover}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={privateMediaVariantURL(coverAssetID, "large")} alt={t("Series cover preview")} />
            </div>
          )}

          <label>{t("Title")}<input maxLength={200} value={title} onChange={(event) => change(setTitle, event.target.value)} /></label>
          <label>{t("URL slug")}<input maxLength={180} value={slug} onChange={(event) => change(setSlug, event.target.value)} /></label>
          <label>{t("Description")}<textarea rows={6} maxLength={1200} value={description} onChange={(event) => change(setDescription, event.target.value)} /></label>
          <label>{t("Status")}
            <select value={status} onChange={(event) => { setStatus(event.target.value as typeof status); markDirty(); }}>
              <option value="active">{t("Active")}</option>
              <option value="complete">{t("Complete")}</option>
              <option value="archived">{t("Archived")}</option>
            </select>
          </label>

          <div className={styles.mediaPanel}>
            <strong>{t("Cover")}</strong>
            <span>{t("JPEG, PNG or WebP.")}</span>
            <label className={styles.uploadButton}>
              {t(mediaBusy ? "Processing cover…" : coverAssetID ? "Replace cover" : "Upload cover")}
              <input type="file" accept="image/jpeg,image/png,image/webp" disabled={mediaBusy} onChange={(event) => {
                const file = event.currentTarget.files?.[0];
                event.currentTarget.value = "";
                if (file) void uploadCover(file);
              }} />
            </label>
            {coverAssetID && <button type="button" onClick={() => { setCoverAssetID(""); markDirty(); }}>{t("Remove cover")}</button>}
          </div>

          <div className={styles.dangerZone}>
            <div><strong>{t("Archive series")}</strong><span>{t("Posts stay intact; their series membership is removed.")}</span></div>
            <button type="button" onClick={() => void archiveSeries()} disabled={mediaBusy || account.state.kind !== "ready"}>{t("Archive")}</button>
          </div>
          {actionError && <p className={styles.error} role="alert">{t(actionError.key, actionError.values)}</p>}
        </main>
      </div>
    </section>
  );
}
