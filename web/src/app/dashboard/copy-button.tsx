"use client";

import { useState } from "react";
import styles from "./dashboard-surfaces.module.css";

export function CopyButton({ value, label = "Copy" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return <button className={styles.button} type="button" onClick={async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      setCopied(false);
    }
  }}>{copied ? "Copied" : label}</button>;
}
