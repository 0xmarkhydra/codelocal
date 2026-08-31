"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isBlogSeriesResource } from "@/lib/contracts/blog";
import { privateMediaVariantURL, uploadMediaAsset } from "@/lib/media-upload";
import { useDashboardResource } from "../use-dashboard-resource";
import dashboard from "../dashboard.module.css";
import styles from "./series-editor.module.css";

type SaveState = "idle" | "saving" | "saved" | "error";

export function SeriesEditor({ seriesID }: { seriesID: string }) {
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
  const [actionError, setActionError] = useState("");
  const editVersion = useRef(0);
  const saveQueue = useRef<Promise<boolean>>(Promise.resolve(true));

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
    if (!series || !dirty || account.state.kind !== "ready") return Promise.resolve(true);
    const snapshot = payload;
    const version = editVersion.current;
    const csrf = account.state.value.csrf;

    const run = async () => {
      setSaveState("saving");
      setActionError("");
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
            setActionError(response.status === 409 ? "That series URL is already in use." : `Save failed (${response.status}).`);
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
      } catch (error) {
        if (editVersion.current === version) {
          setSaveState("error");
          setActionError(error instanceof Error ? error.message : "Save failed.");
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
    setActionError("");
    try {
      const asset = await uploadMediaAsset(file, account.state.value.csrf);
      setCoverAssetID(asset.id);
      markDirty();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Cover upload failed.");
    } finally {
      setMediaBusy(false);
    }
  }

  async function archiveSeries() {
    if (!series || account.state.kind !== "ready") return;
    if (!(await save())) return;
    setActionError("");
    try {
      const response = await fetch(`/api/v1/blog/series/${encodeURIComponent(series.id)}`, {
        method: "DELETE",
        credentials: "same-origin",
        headers: { Accept: "application/json", "X-CSRF-Token": account.state.value.csrf },
      });
      if (!response.ok) {
        setActionError(`Unable to archive series (${response.status}).`);
        return;
      }
      router.push("/dashboard/blogs");
      router.refresh();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Unable to archive series.");
    }
  }

  if (resource.state.kind === "loading") {
    return <section className={dashboard.content}><div className={styles.state}>Loading series…</div></section>;
  }
  if (resource.state.kind !== "ready" || !series) {
    const message = resource.state.kind === "error" ? resource.state.message : "This series is not available.";
    return <section className={dashboard.content}><div className={styles.state}>{message}</div></section>;
  }

  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.topbar}>
          <Link href="/dashboard/blogs">← Blogs</Link>
          <span aria-live="polite">{mediaBusy ? "Processing cover…" : saveState === "saving" ? "Saving…" : saveState === "saved" ? "Saved" : saveState === "error" ? "Save failed" : dirty ? "Unsaved" : ""}</span>
          <div>
            {series.status !== "archived" && <Link href={`/blogs/series/${series.slug}`}>View</Link>}
            <button type="button" onClick={() => void save()} disabled={!dirty || mediaBusy || saveState === "saving"}>Save</button>
          </div>
        </header>

        <main className={styles.form}>
          <div className={styles.heading}>
            <span>Series</span>
            <h1>{title || "Untitled series"}</h1>
            <p>Build an ordered learning path. Posts choose this series and their part number from the normal Blog editor.</p>
          </div>

          {coverAssetID && (
            <div className={styles.cover}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={privateMediaVariantURL(coverAssetID, "large")} alt="Series cover preview" />
            </div>
          )}

          <label>Title<input maxLength={200} value={title} onChange={(event) => change(setTitle, event.target.value)} /></label>
          <label>URL slug<input maxLength={180} value={slug} onChange={(event) => change(setSlug, event.target.value)} /></label>
          <label>Description<textarea rows={6} maxLength={1200} value={description} onChange={(event) => change(setDescription, event.target.value)} /></label>
          <label>Status
            <select value={status} onChange={(event) => { setStatus(event.target.value as typeof status); markDirty(); }}>
              <option value="active">Active</option>
              <option value="complete">Complete</option>
              <option value="archived">Archived</option>
            </select>
          </label>

          <div className={styles.mediaPanel}>
            <strong>Cover</strong>
            <span>Uses the same durable S3/WebP media pipeline as Blog posts.</span>
            <label className={styles.uploadButton}>
              {mediaBusy ? "Processing…" : coverAssetID ? "Replace cover" : "Upload cover"}
              <input type="file" accept="image/jpeg,image/png,image/webp" disabled={mediaBusy} onChange={(event) => {
                const file = event.currentTarget.files?.[0];
                event.currentTarget.value = "";
                if (file) void uploadCover(file);
              }} />
            </label>
            {coverAssetID && <button type="button" onClick={() => { setCoverAssetID(""); markDirty(); }}>Remove cover</button>}
          </div>

          <div className={styles.dangerZone}>
            <div><strong>Archive series</strong><span>Posts stay intact; their series membership is removed.</span></div>
            <button type="button" onClick={() => void archiveSeries()}>Archive</button>
          </div>
          {actionError && <p className={styles.error} role="alert">{actionError}</p>}
        </main>
      </div>
    </section>
  );
}
