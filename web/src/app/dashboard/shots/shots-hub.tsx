"use client";

import Link from "next/link";
import { DragEvent, useCallback, useEffect, useRef, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import {
  isScreenshotShareResource,
  isScreenshotSharesResource,
  type ScreenshotShare,
} from "@/lib/contracts/shots";
import { privateMediaVariantURL, uploadMediaAsset } from "@/lib/media-upload";
import { AppIcon } from "../app-icon";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./shots.module.css";

const SUPPORTED_IMAGE_TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);
const MAX_IMAGE_BYTES = 25 * 1024 * 1024;

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${Math.max(1, Math.round(value / 1024))} KB`;
  return `${(value / (1024 * 1024)).toFixed(value >= 10 * 1024 * 1024 ? 0 : 1)} MB`;
}

function formatDate(value: number) {
  return new Intl.DateTimeFormat("en", {
    month: "short", day: "numeric", hour: "2-digit", minute: "2-digit",
  }).format(new Date(value));
}

function fileError(file: File) {
  if (!SUPPORTED_IMAGE_TYPES.has(file.type)) return "Use a PNG, JPEG, or WebP image.";
  if (file.size <= 0 || file.size > MAX_IMAGE_BYTES) return "Image must be smaller than 25 MB.";
  return "";
}

export function ShotsHub() {
  const { state: accountState } = useDashboardResource("/api/v1/account", isAccountResource);
  const { state: sharesState, retry: retryShares } = useDashboardResource("/api/v1/shots", isScreenshotSharesResource);
  const fileInput = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [previewURL, setPreviewURL] = useState("");
  const [latestShare, setLatestShare] = useState<ScreenshotShare>();
  const [latestAssetID, setLatestAssetID] = useState("");
  const [copiedID, setCopiedID] = useState("");
  const [revokingID, setRevokingID] = useState("");

  useEffect(() => () => {
    if (previewURL) URL.revokeObjectURL(previewURL);
  }, [previewURL]);

  const publishFile = useCallback(async (file: File) => {
    if (uploading) return;
    if (accountState.kind !== "ready") {
      setError("Sign in again before sharing an image.");
      return;
    }
    const validationError = fileError(file);
    if (validationError) {
      setError(validationError);
      return;
    }

    setUploading(true);
    setError("");
    setLatestShare(undefined);
    setLatestAssetID("");
    setPreviewURL(URL.createObjectURL(file));
    try {
      const asset = await uploadMediaAsset(file, accountState.value.csrf, { preserveOriginal: true });
      const response = await fetch("/api/v1/shots", {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": accountState.value.csrf,
        },
        body: JSON.stringify({ assetId: asset.id }),
      });
      const body: unknown = await response.json().catch(() => null);
      if (!response.ok || !isScreenshotShareResource(body)) {
        throw new Error(`Unable to create the public link (${response.status}).`);
      }
      setLatestAssetID(asset.id);
      setLatestShare(body.share);
      retryShares();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to share this image.");
    } finally {
      setUploading(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  }, [accountState, retryShares, uploading]);

  useEffect(() => {
    function pasteImage(event: ClipboardEvent) {
      const target = event.target as HTMLElement | null;
      if (target?.matches("input, textarea, [contenteditable='true']")) return;
      const file = Array.from(event.clipboardData?.files ?? []).find((item) => item.type.startsWith("image/"));
      if (!file) return;
      event.preventDefault();
      void publishFile(file);
    }
    window.addEventListener("paste", pasteImage);
    return () => window.removeEventListener("paste", pasteImage);
  }, [publishFile]);

  function dropImage(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    const file = Array.from(event.dataTransfer.files).find((item) => item.type.startsWith("image/"));
    if (file) void publishFile(file);
  }

  async function copyLink(share: ScreenshotShare) {
    try {
      await navigator.clipboard.writeText(share.url);
      setCopiedID(share.id);
      window.setTimeout(() => setCopiedID((current) => current === share.id ? "" : current), 1500);
    } catch {
      setError("Your browser blocked clipboard access. Select and copy the link manually.");
    }
  }

  async function revokeShare(share: ScreenshotShare) {
    if (accountState.kind !== "ready" || revokingID) return;
    if (!window.confirm("Revoke this public link? The underlying image stays in your CodeLocal media library.")) return;
    setRevokingID(share.id);
    setError("");
    try {
      const response = await fetch(`/api/v1/shots/${encodeURIComponent(share.id)}`, {
        method: "DELETE",
        credentials: "same-origin",
        headers: { Accept: "application/json", "X-CSRF-Token": accountState.value.csrf },
      });
      if (!response.ok) throw new Error(`Unable to revoke the link (${response.status}).`);
      if (latestShare?.id === share.id) setLatestShare(undefined);
      retryShares();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to revoke this link.");
    } finally {
      setRevokingID("");
    }
  }

  const recentShares = sharesState.kind === "ready" ? sharesState.value.shares : [];

  return (
    <div className={styles.workspace}>
      <section className={styles.uploadPanel}>
        <div
          className={styles.dropZone}
          data-dragging={dragging}
          data-uploading={uploading}
          onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
          onDragOver={(event) => event.preventDefault()}
          onDrop={dropImage}
        >
          <span className={styles.dropIcon} aria-hidden="true"><AppIcon name="upload" size={25} /></span>
          <span className={styles.kicker}>Instant public link</span>
          <h2>{uploading ? "Preparing your shot…" : dragging ? "Drop to upload" : "Drop a screenshot to share"}</h2>
          <p>Choose an image, drag it here, or paste directly from your clipboard.</p>
          <button disabled={uploading || accountState.kind !== "ready"} onClick={() => fileInput.current?.click()} type="button">
            <AppIcon name="image" size={15} />
            {uploading ? "Uploading…" : "Choose image"}
          </button>
          <input
            accept="image/png,image/jpeg,image/webp"
            aria-label="Choose screenshot"
            hidden
            onChange={(event) => { const file = event.target.files?.[0]; if (file) void publishFile(file); }}
            ref={fileInput}
            type="file"
          />
          <div className={styles.dropMeta}><span>PNG · JPEG · WEBP</span><span>UP TO 25 MB</span><span>FULL RESOLUTION</span></div>
          {uploading && <span className={styles.progressTrack} aria-label="Uploading image"><i /></span>}
        </div>

        <aside className={styles.resultPanel} aria-live="polite">
          {previewURL || latestAssetID ? (
            <div className={styles.previewFrame}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={latestAssetID ? privateMediaVariantURL(latestAssetID, "large") : previewURL} alt="Screenshot preview" />
              {uploading && <span className={styles.previewBusy}>Processing securely</span>}
            </div>
          ) : (
            <div className={styles.emptyPreview} aria-hidden="true">
              <AppIcon name="image" size={36} />
              <span>Your preview appears here</span>
            </div>
          )}

          {latestShare ? (
            <div className={styles.successCard}>
              <div className={styles.successTitle}><span><AppIcon name="check" size={14} /></span><div><strong>Link ready</strong><small>Anyone with this link can view the shot.</small></div></div>
              <label><span>Public link</span><input aria-label="Public screenshot link" readOnly value={latestShare.url} /></label>
              <div className={styles.resultActions}>
                <button onClick={() => void copyLink(latestShare)} type="button"><AppIcon name="copy" size={14} />{copiedID === latestShare.id ? "Copied" : "Copy link"}</button>
                <Link href={latestShare.url} target="_blank"><AppIcon name="external" size={14} />Open shot</Link>
              </div>
            </div>
          ) : (
            <div className={styles.sharingNote}>
              <span className={styles.kicker}>Designed for sharing</span>
              <ul>
                <li><i />The storage endpoint stays private.</li>
                <li><i />Links are random and excluded from search.</li>
                <li><i />Revoke a link without deleting the source image.</li>
              </ul>
            </div>
          )}
        </aside>
      </section>

      {error && <div className={styles.errorBanner} role="alert"><span>Upload interrupted</span><p>{error}</p><button onClick={() => setError("")} type="button" aria-label="Dismiss error"><AppIcon name="close" size={14} /></button></div>}

      <section className={styles.recentSection}>
        <header className={styles.sectionHeader}>
          <div><span className={styles.kicker}>Your links</span><h2>Recent shots</h2><p>Public links you created, newest first.</p></div>
          <span className={styles.count}>{recentShares.length} active</span>
        </header>

        {sharesState.kind === "loading" && <div className={styles.emptyState}>Loading your shared screenshots…</div>}
        {sharesState.kind === "unauthenticated" && <div className={styles.emptyState}>Sign in again to manage your links.</div>}
        {sharesState.kind === "error" && <div className={styles.emptyState}><p>{sharesState.message}</p><button onClick={retryShares} type="button">Try again</button></div>}
        {sharesState.kind === "ready" && recentShares.length === 0 && <div className={styles.emptyState}>No public links yet. Your first shared screenshot will appear here.</div>}
        {recentShares.length > 0 && (
          <div className={styles.shareGrid}>
            {recentShares.map((share) => (
              <article className={styles.shareCard} key={share.id}>
                <Link className={styles.shareThumb} href={share.url} target="_blank">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img alt="Shared screenshot thumbnail" loading="lazy" src={share.thumbnailUrl} />
                </Link>
                <div className={styles.shareDetails}>
                  <span>{share.width} × {share.height} · {formatBytes(share.size)}</span>
                  <strong title={share.url}>{share.url.replace(/^https?:\/\//, "")}</strong>
                  <time dateTime={new Date(share.createdAt).toISOString()}>{formatDate(share.createdAt)}</time>
                </div>
                <div className={styles.shareActions}>
                  <button aria-label="Copy public link" onClick={() => void copyLink(share)} title="Copy link" type="button"><AppIcon name={copiedID === share.id ? "check" : "copy"} size={14} /></button>
                  <Link aria-label="Open public shot" href={share.url} target="_blank" title="Open shot"><AppIcon name="external" size={14} /></Link>
                  <button aria-label="Revoke public link" className={styles.revokeButton} disabled={Boolean(revokingID)} onClick={() => void revokeShare(share)} title="Revoke link" type="button"><AppIcon name="trash" size={14} /></button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
