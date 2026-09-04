"use client";

import { useState } from "react";
import Link from "next/link";
import { AppIcon } from "../../dashboard/app-icon";
import styles from "./share.module.css";

export function ShareActions({ shareURL, downloadURL }: { shareURL: string; downloadURL: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(shareURL);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className={styles.actions}>
      <button onClick={() => void copy()} type="button"><AppIcon name={copied ? "check" : "copy"} size={15} />{copied ? "Copied" : "Copy link"}</button>
      <a href={downloadURL}><AppIcon name="download" size={15} />Download</a>
      <Link href="/dashboard/shots"><AppIcon name="upload" size={15} />Share another</Link>
    </div>
  );
}
