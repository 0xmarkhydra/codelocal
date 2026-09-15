"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChangeEvent, FormEvent, useCallback, useEffect, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { privateMediaVariantURL, uploadMediaAsset } from "@/lib/media-upload";
import { useTranslations } from "@/lib/i18n/provider";
import { AppIcon } from "../app-icon";
import { useDashboardResource } from "../use-dashboard-resource";
import { ForumComment, ForumSeverity, ForumStatus, ForumTopic, formatForumTime, isForumTopic, kindLabels, severityLabels, statusLabels } from "./forum-types";
import styles from "./forums.module.css";

type TopicResponse = { topic: ForumTopic; comments: ForumComment[]; isAdmin: boolean };

type ReviewDraft = {
  status: ForumStatus;
  severity: ForumSeverity;
  githubIssueUrl: string;
  githubIssueNumber: string;
  githubPrUrl: string;
  resolutionNote: string;
};

function isComment(value: unknown): value is ForumComment {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return typeof item.id === "string" && typeof item.topicId === "string" && typeof item.authorEmail === "string" && typeof item.body === "string" && Array.isArray(item.assetIds) && item.assetIds.every((assetID) => typeof assetID === "string") && typeof item.createdAt === "number";
}

function validTopicResponse(value: unknown): value is TopicResponse {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return isForumTopic(item.topic) && Array.isArray(item.comments) && item.comments.every(isComment) && typeof item.isAdmin === "boolean";
}

function reviewFromTopic(topic: ForumTopic): ReviewDraft {
  return {
    status: topic.status,
    severity: topic.severity ?? "",
    githubIssueUrl: topic.githubIssueUrl ?? "",
    githubIssueNumber: topic.githubIssueNumber ? String(topic.githubIssueNumber) : "",
    githubPrUrl: topic.githubPrUrl ?? "",
    resolutionNote: topic.resolutionNote ?? "",
  };
}

export function ForumTopicLive({ topicID }: { topicID: string }) {
  const { locale, t, message } = useTranslations();
  const router = useRouter();
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const readyAccount = account.state.kind === "ready" ? account.state.value : undefined;
  const [topic, setTopic] = useState<ForumTopic | null>(null);
  const [comments, setComments] = useState<ForumComment[]>([]);
  const [isAdmin, setIsAdmin] = useState(false);
  const [review, setReview] = useState<ReviewDraft | null>(null);
  const [reply, setReply] = useState("");
  const [replyAssetIDs, setReplyAssetIDs] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [moderationReason, setModerationReason] = useState("");

  const fetchTopic = useCallback(async () => {
    const response = await fetch(`/api/v1/forums/topics/${encodeURIComponent(topicID)}`, { cache: "no-store", credentials: "same-origin" });
    if (!response.ok) throw new Error(response.status === 404 ? "Topic not found" : "Unable to load topic");
    const payload: unknown = await response.json();
    if (!validTopicResponse(payload)) throw new Error("Invalid forum response");
    return payload;
  }, [topicID]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const payload = await fetchTopic();
      setTopic(payload.topic);
      setComments(payload.comments);
      setIsAdmin(payload.isAdmin);
      setReview(reviewFromTopic(payload.topic));
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Unable to load topic"));
    } finally {
      setLoading(false);
    }
  }, [fetchTopic, message, t]);

  useEffect(() => {
    let cancelled = false;
    fetchTopic()
      .then((payload) => {
        if (cancelled) return;
        setTopic(payload.topic);
        setComments(payload.comments);
        setIsAdmin(payload.isAdmin);
        setReview(reviewFromTopic(payload.topic));
        setError("");
      })
      .catch((reason: unknown) => {
        if (!cancelled) setError(reason instanceof Error ? message(reason.message) : t("Unable to load topic"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [fetchTopic, message, t]);

  async function addReplyImages(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.currentTarget.files ?? []);
    event.currentTarget.value = "";
    if (!readyAccount?.csrf || files.length === 0 || uploading) return;
    const remaining = Math.max(0, 4 - replyAssetIDs.length);
    if (remaining === 0) return;
    setUploading(true);
    setError("");
    try {
      const next = [...replyAssetIDs];
      for (const file of files.slice(0, remaining)) {
        const asset = await uploadMediaAsset(file, readyAccount.csrf);
        if (!next.includes(asset.id)) next.push(asset.id);
      }
      setReplyAssetIDs(next);
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Could not upload image"));
    } finally {
      setUploading(false);
    }
  }

  async function postReply(event: FormEvent) {
    event.preventDefault();
    if (!readyAccount?.csrf || !reply.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`/api/v1/forums/topics/${encodeURIComponent(topicID)}/comments`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": readyAccount.csrf },
        body: JSON.stringify({ body: reply, assetIds: replyAssetIDs }),
      });
      if (!response.ok) throw new Error("Could not publish reply");
      setReply("");
      setReplyAssetIDs([]);
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Could not publish reply"));
    } finally {
      setBusy(false);
    }
  }

  async function toggleVote() {
    if (!readyAccount?.csrf || !topic || busy) return;
    setBusy(true);
    try {
      const response = await fetch(`/api/v1/forums/topics/${encodeURIComponent(topicID)}/vote`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-CSRF-Token": readyAccount.csrf },
      });
      if (!response.ok) throw new Error("Could not update vote");
      const payload = await response.json() as { voted?: boolean; voteCount?: number };
      if (typeof payload.voted !== "boolean" || typeof payload.voteCount !== "number") throw new Error("Invalid vote response");
      const voted = payload.voted;
      const voteCount = payload.voteCount;
      setTopic((current) => current ? { ...current, votedByViewer: voted, voteCount } : current);
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Could not update vote"));
    } finally {
      setBusy(false);
    }
  }

  async function deleteTopic() {
    if (!readyAccount?.csrf || !topic || !isAdmin || busy || !moderationReason.trim()) return;
    if (!window.confirm(t("Delete “{title}” and all of its replies? This will hide it from the public forum.", { title: topic.title }))) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`/api/v1/admin/forums/topics/${encodeURIComponent(topicID)}`, {
        method: "DELETE",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": readyAccount.csrf },
        body: JSON.stringify({ reason: moderationReason.trim() }),
      });
      if (!response.ok) throw new Error("Could not delete topic");
      router.push("/dashboard/forums");
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Could not delete topic"));
      setBusy(false);
    }
  }

  async function deleteComment(commentID: string) {
    if (!readyAccount?.csrf || !isAdmin || busy || !moderationReason.trim()) return;
    if (!window.confirm(t("Delete this reply? It will disappear from the forum immediately."))) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`/api/v1/admin/forums/topics/${encodeURIComponent(topicID)}/comments/${encodeURIComponent(commentID)}`, {
        method: "DELETE",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": readyAccount.csrf },
        body: JSON.stringify({ reason: moderationReason.trim() }),
      });
      if (!response.ok) throw new Error("Could not delete reply");
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Could not delete reply"));
    } finally {
      setBusy(false);
    }
  }

  async function saveReview(event: FormEvent) {
    event.preventDefault();
    if (!readyAccount?.csrf || !review || busy) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`/api/v1/admin/forums/topics/${encodeURIComponent(topicID)}`, {
        method: "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": readyAccount.csrf },
        body: JSON.stringify({
          ...review,
          githubIssueNumber: review.githubIssueNumber.trim() ? Number(review.githubIssueNumber) : 0,
        }),
      });
      if (!response.ok) throw new Error("Could not save admin review");
      const payload = await response.json() as { topic?: ForumTopic };
      if (!payload.topic || !isForumTopic(payload.topic)) throw new Error("Invalid review response");
      setTopic(payload.topic);
      setReview(reviewFromTopic(payload.topic));
    } catch (reason) {
      setError(reason instanceof Error ? message(reason.message) : t("Could not save admin review"));
    } finally {
      setBusy(false);
    }
  }

  if (loading) return <div className={styles.page}><div className={styles.empty}>{t("Loading topic…")}</div></div>;
  if (!topic) return <div className={styles.page}><div className={styles.error}>{error || t("Topic not found")}</div></div>;

  return (
    <div className={styles.page}>
      <div className={styles.topicPage}>
        <Link className={styles.backLink} href="/dashboard/forums"><AppIcon name="chevron-left" size={14} />{t("Back to Forums")}</Link>

        {error && <div className={styles.error}><AppIcon name="shield" size={17} />{error}<button type="button" onClick={() => setError("")}>{t("Dismiss")}</button></div>}

        <article className={styles.detailHero}>
          <div className={styles.topicMeta}>
            <span className={styles.kindBadge} data-kind={topic.kind}>{t(kindLabels[topic.kind])}</span>
            <span className={styles.statusBadge} data-status={topic.status}>{t(statusLabels[topic.status])}</span>
            {topic.kind === "bug" && topic.severity && <span className={styles.severity} data-severity={topic.severity}>{t(severityLabels[topic.severity])}</span>}
          </div>
          <h1>{topic.title}</h1>
          <p>{topic.body}</p>
          {topic.assetIds.length > 0 && <div className={styles.detailGallery}>{topic.assetIds.map((assetID, index) => <a href={privateMediaVariantURL(assetID, "large")} target="_blank" rel="noreferrer" key={assetID}><Image src={privateMediaVariantURL(assetID, "medium")} alt={t("Attachment {count}", { count: index + 1 })} width={900} height={600} unoptimized /></a>)}</div>}
          <div className={styles.detailMeta}><span>{t("Opened by {email}", { email: topic.authorEmail })}</span><span>{formatForumTime(topic.createdAt, locale)}</span><span>{t("{count} replies", { count: topic.commentCount })}</span></div>
          <div className={styles.tags}>{topic.tags.map((tag) => <span key={tag}>#{tag}</span>)}</div>

          {topic.kind === "bug" && (
            <div className={styles.bugDetail}>
              <article><strong>{t("Version")}</strong><p>{topic.version || t("Not provided")}</p></article>
              <article><strong>{t("Environment")}</strong><p>{topic.environment || t("Not provided")}</p></article>
              <article><strong>{t("Steps to reproduce")}</strong><p>{topic.reproductionSteps || t("Not provided")}</p></article>
              <article><strong>{t("Expected behavior")}</strong><p>{topic.expectedBehavior || t("Not provided")}</p></article>
              <article><strong>{t("Actual behavior")}</strong><p>{topic.actualBehavior || t("Not provided")}</p></article>
            </div>
          )}

          {(topic.githubIssueUrl || topic.githubPrUrl) && <div className={styles.issueLinks}>
            {topic.githubIssueUrl && <a href={topic.githubIssueUrl} target="_blank" rel="noreferrer"><AppIcon name="external" size={13} />{t("GitHub Issue")}{topic.githubIssueNumber ? ` #${topic.githubIssueNumber}` : ""}</a>}
            {topic.githubPrUrl && <a href={topic.githubPrUrl} target="_blank" rel="noreferrer"><AppIcon name="external" size={13} />{t("Pull Request / Fix")}</a>}
          </div>}

          {topic.resolutionNote && <div className={styles.resolution}><strong>{t("Resolution")}</strong><br />{topic.resolutionNote}</div>}

          <div className={styles.detailActions}>
            <button className={styles.voteButton} data-active={topic.votedByViewer || undefined} disabled={busy || !readyAccount} type="button" onClick={() => void toggleVote()}>
              <AppIcon name="plus" size={14} />{t(topic.votedByViewer ? "Voted" : "Upvote")} · {topic.voteCount}
            </button>
          </div>
        </article>

        <div className={styles.contentGrid}>
          <section className={styles.replies}>
            <h2 className={styles.sectionTitle}>{t("Replies · {count}", { count: comments.length })}</h2>
            <div className={styles.replyList}>
              {comments.length === 0 && <div className={styles.empty}>{t("No replies yet. Add context or a workaround.")}</div>}
              {comments.map((comment) => <article className={styles.replyCard} key={comment.id}>
                <header><strong>{comment.authorEmail}</strong><div className={styles.replyHeaderActions}><span>{formatForumTime(comment.createdAt, locale)}</span>{isAdmin && <button className={styles.deleteInlineButton} type="button" disabled={busy || !moderationReason.trim()} onClick={() => void deleteComment(comment.id)}>{t("Delete reply")}</button>}</div></header>
                <p>{comment.body}</p>
                {comment.assetIds.length > 0 && <div className={styles.replyGallery}>{comment.assetIds.map((assetID, index) => <a href={privateMediaVariantURL(assetID, "large")} target="_blank" rel="noreferrer" key={assetID}><Image src={privateMediaVariantURL(assetID, "thumb")} alt={t("Reply attachment {count}", { count: index + 1 })} width={180} height={120} unoptimized /></a>)}</div>}
              </article>)}
            </div>
            <form className={styles.replyComposer} onSubmit={postReply}>
              <label><span>{t("Reply")}</span><textarea required rows={4} value={reply} onChange={(event) => setReply(event.target.value)} placeholder={t("Share a fix, workaround, reproduction detail, or answer…")} /></label>
              <div className={styles.attachments}>
                <div className={styles.attachmentHead}><div><strong>{t("Images")}</strong><span>{t("Up to 4 screenshots")}</span></div><label className={styles.fileButton}>{uploading ? t("Uploading…") : t("Add images")}<input type="file" accept="image/jpeg,image/png,image/webp" multiple disabled={uploading || !readyAccount || replyAssetIDs.length >= 4} onChange={(event) => void addReplyImages(event)} /></label></div>
                {replyAssetIDs.length > 0 && <div className={styles.attachmentGrid}>{replyAssetIDs.map((assetID) => <div className={styles.attachment} key={assetID}><Image src={privateMediaVariantURL(assetID, "thumb")} alt={t("Reply image preview")} width={180} height={120} unoptimized /><button type="button" aria-label={t("Remove image")} onClick={() => setReplyAssetIDs((current) => current.filter((id) => id !== assetID))}>×</button></div>)}</div>}
              </div>
              <footer><button className={styles.primaryButton} disabled={busy || uploading || !readyAccount || !reply.trim()} type="submit">{busy ? t("Posting…") : uploading ? t("Uploading…") : t("Post reply")}</button></footer>
            </form>
          </section>

          {isAdmin && review && (
            <form className={styles.adminPanel} onSubmit={saveReview}>
              <span className={styles.eyebrow}>{t("Admin workflow")}</span>
              <h2>{t("Review & engineering handoff")}</h2>
              <p>{t("Validate the report, link the GitHub issue/PR, then move the topic through the delivery status.")}</p>
              <label><span>{t("Status")}</span><select value={review.status} onChange={(event) => setReview((current) => current ? { ...current, status: event.target.value as ForumStatus } : current)}>{Object.entries(statusLabels).map(([value, label]) => <option key={value} value={value}>{t(label)}</option>)}</select></label>
              {topic.kind === "bug" && <label><span>{t("Severity")}</span><select value={review.severity} onChange={(event) => setReview((current) => current ? { ...current, severity: event.target.value as ForumSeverity } : current)}><option value="">{t("Unspecified")}</option><option value="low">{t("Low")}</option><option value="medium">{t("Medium")}</option><option value="high">{t("High")}</option><option value="critical">{t("Critical")}</option></select></label>}
              <label><span>{t("GitHub issue #")}</span><input inputMode="numeric" value={review.githubIssueNumber} onChange={(event) => setReview((current) => current ? { ...current, githubIssueNumber: event.target.value.replace(/\D/g, "") } : current)} placeholder="123" /></label>
              <label><span>{t("GitHub issue URL")}</span><input type="url" value={review.githubIssueUrl} onChange={(event) => setReview((current) => current ? { ...current, githubIssueUrl: event.target.value } : current)} placeholder="https://github.com/codelocal-cloud/codelocal/issues/123" /></label>
              <label><span>{t("PR / fix URL")}</span><input type="url" value={review.githubPrUrl} onChange={(event) => setReview((current) => current ? { ...current, githubPrUrl: event.target.value } : current)} placeholder="https://github.com/codelocal-cloud/codelocal/pull/123" /></label>
              <label><span>{t("Resolution note")}</span><textarea rows={4} value={review.resolutionNote} onChange={(event) => setReview((current) => current ? { ...current, resolutionNote: event.target.value } : current)} placeholder={t("What was fixed, merged, or why this was closed")} /></label>
              <button disabled={busy} type="submit">{busy ? t("Saving…") : t("Save review")}</button>
              <div className={styles.dangerZone}>
                <strong>{t("Moderation")}</strong>
                <p>{t("Deletion is a soft delete. The content disappears immediately while the audit reason is retained.")}</p>
                <label><span>{t("Deletion reason")}</span><textarea rows={3} maxLength={500} value={moderationReason} onChange={(event) => setModerationReason(event.target.value)} placeholder={t("Why is this content being removed?")} /></label>
                <button className={styles.dangerButton} disabled={busy || !moderationReason.trim()} type="button" onClick={() => void deleteTopic()}>{t("Delete topic")}</button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
