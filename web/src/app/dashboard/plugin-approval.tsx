"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "@/lib/i18n/provider";
import styles from "./chat-action-summary.module.css";

export function PluginApproval({ id, busy }: { id: string; busy: boolean }) {
  const { t } = useTranslations();
  const [pending, setPending] = useState(false);
  const [finished, setFinished] = useState(false);
  const [result, setResult] = useState("");
  const [details, setDetails] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void fetch(`/api/v1/plugin-approvals/${encodeURIComponent(id)}`, { credentials: "same-origin", cache: "no-store", signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error("approval_unavailable");
        const value: unknown = await response.json();
        if (!value || typeof value !== "object" || !("id" in value) || value.id !== id) throw new Error("invalid_approval");
        setDetails(JSON.stringify(value, null, 2));
      }).catch(() => { if (!controller.signal.aborted) setFinished(true); });
    return () => controller.abort();
  }, [id]);
  async function decide(approve: boolean) {
    setPending(true);
    try {
      const account = await fetch("/api/v1/account", { credentials: "same-origin", cache: "no-store" });
      if (!account.ok) throw new Error(t("Sign in required"));
      const auth = await account.json() as { csrf?: unknown };
      if (typeof auth.csrf !== "string" || !auth.csrf) throw new Error(t("Security token unavailable. Reload the page and try again."));
      const response = await fetch(`/api/v1/plugin-approvals/${encodeURIComponent(id)}`, {
        method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": auth.csrf },
        body: JSON.stringify({ approve }),
      });
      setFinished(true);
      const payload: unknown = await response.json();
      setResult(JSON.stringify(payload, null, 2));
    } catch (error) {
      setFinished(true);
      setResult(`${error instanceof Error ? error.message : t("Unable to connect")} ${t("The operation may have run. Check the service before trying again.")}`);
    } finally { setPending(false); }
  }
  return <div>
    {details && <pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", maxHeight: 280, overflow: "auto" }}>{details}</pre>}
    <div className={styles.approvalRow}>
      <span>{t("Review the exact tool and arguments before allowing this operation.")}</span>
      <button type="button" disabled={busy || pending || finished || !details} onClick={() => void decide(false)}>{t("Deny")}</button>
      <button type="button" disabled={busy || pending || finished || !details} onClick={() => void decide(true)}>{t("Allow once")}</button>
    </div>
    {result && <pre role="status" style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", maxHeight: 320, overflow: "auto" }}>{result}</pre>}
  </div>;
}
