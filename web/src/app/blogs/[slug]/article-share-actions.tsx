"use client";

import { useState } from "react";
import styles from "./article-share-actions.module.css";

type Props = {
  slug: string;
  title: string;
};

type Feedback = "" | "shared" | "copied" | "short-copied" | "error";

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand("copy");
  textarea.remove();
}

export function ArticleShareActions({ slug, title }: Props) {
  const [feedback, setFeedback] = useState<Feedback>("");

  function absoluteURL(path: string) {
    return new URL(path, window.location.origin).toString();
  }

  async function share() {
    const url = absoluteURL(`/blogs/${encodeURIComponent(slug)}`);
    try {
      if (navigator.share) {
        await navigator.share({ title, url });
        setFeedback("shared");
      } else {
        await copyText(url);
        setFeedback("copied");
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setFeedback("error");
    }
  }

  async function copyShortLink() {
    try {
      await copyText(absoluteURL(`/b/${encodeURIComponent(slug)}`));
      setFeedback("short-copied");
    } catch {
      setFeedback("error");
    }
  }

  const message = feedback === "shared"
    ? "Shared"
    : feedback === "copied"
      ? "Link copied"
      : feedback === "short-copied"
        ? "Short link copied"
        : feedback === "error"
          ? "Could not copy link"
          : "";

  return (
    <div className={styles.actions} aria-label="Article sharing">
      <button type="button" onClick={() => void share()}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15.5V4m0 0 4 4m-4-4-4 4M5 13.5v5.25h14V13.5" /></svg>
        Share
      </button>
      <button type="button" onClick={() => void copyShortLink()}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9.25 14.75-1.6 1.6a3.55 3.55 0 1 1-5.02-5.02l3-3A3.55 3.55 0 0 1 10.65 8" /><path d="m14.75 9.25 1.6-1.6a3.55 3.55 0 1 1 5.02 5.02l-3 3A3.55 3.55 0 0 1 13.35 16" /><path d="m8.6 15.4 6.8-6.8" /></svg>
        Short link
      </button>
      <span className={styles.feedback} aria-live="polite">{message}</span>
    </div>
  );
}
