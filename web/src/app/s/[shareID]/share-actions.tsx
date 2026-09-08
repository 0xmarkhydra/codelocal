"use client";

import { useState } from "react";
import Link from "next/link";
import { useTranslations } from "@/lib/i18n/provider";
import { AppIcon } from "../../dashboard/app-icon";
import styles from "./share.module.css";

export function ShareActions({ shareURL, downloadURL }: { shareURL: string; downloadURL: string }) {
  const { t } = useTranslations();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  async function copy() {
    setCopyFailed(false);
    try {
      await navigator.clipboard.writeText(shareURL);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  }

  return (
    <div className={styles.actions}>
      <button onClick={() => void copy()} type="button"><AppIcon name={copied ? "check" : "copy"} size={15} />{t(copied ? "Copied" : "Copy link")}</button>
      <a href={downloadURL}><AppIcon name="download" size={15} />{t("Download")}</a>
      <Link href="/dashboard/shots"><AppIcon name="upload" size={15} />{t("Share another")}</Link>
      {copyFailed && <div className={styles.copyError}>
        <p role="status">{t("Your browser blocked clipboard access. Select and copy the link manually.")}</p>
        <input aria-label={t("Public screenshot link")} readOnly value={shareURL} onFocus={(event) => event.currentTarget.select()} />
      </div>}
    </div>
  );
}
