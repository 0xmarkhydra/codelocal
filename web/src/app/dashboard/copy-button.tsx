"use client";

import { useState } from "react";
import styles from "./dashboard-surfaces.module.css";
import { useTranslations } from "@/lib/i18n/provider";

export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useTranslations();
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  return <><button className={styles.button} type="button" onClick={async () => {
    try {
      await navigator.clipboard.writeText(value);
      setFailed(false);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      setCopied(false);
      setFailed(true);
    }
  }}>{copied ? t("Copied") : label ?? t("Copy")}</button>{failed && <span role="status">{t("Copy failed. Try again.")}</span>}</>;
}
