"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import styles from "./dashboard.module.css";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey, MessageValues } from "@/lib/i18n/messages";

type MutationState = { kind: "idle" | "saving" | "success" | "error"; message?: { key: MessageKey; values?: MessageValues } | { text: string } };

type ResourceMutationButtonProps = {
  endpoint: string;
  csrf?: string;
  label: string;
  confirmMessage: string;
  disabled?: boolean;
  onSuccess: () => void;
};

function safeRedirectTarget(value: string) {
  try {
    const url = new URL(value, window.location.origin);
    if (url.origin !== window.location.origin) return "/login";
    return `${url.pathname}${url.search}${url.hash}`;
  } catch {
    return "/login";
  }
}

async function responseMessage(response: Response): Promise<NonNullable<MutationState["message"]>> {
  try {
    const payload = await response.json() as { message?: unknown; error?: unknown };
    switch (payload.error) {
      case "invalid_csrf": return { key: "Security token expired. Reload the page and try again." };
      case "workspace_runtime_offline": return { key: "Start CodeLocal.Cloud on that machine before removing workspace access." };
      case "device_not_found": return { key: "This device is no longer available to revoke." };
      case "workspace_not_found": return { key: "This workspace is no longer authorized." };
      case "device_already_revoked": return { key: "This device has already been revoked." };
    }
    if (typeof payload.message === "string" && payload.message.length <= 240) return { text: payload.message };
  } catch {
    // Non-JSON responses still carry an HTTP status.
  }
  return { key: "CodeLocal.Cloud rejected this change ({status}).", values: { status: String(response.status) } };
}

export function ResourceMutationButton({ endpoint, csrf, label, confirmMessage, disabled, onSuccess }: ResourceMutationButtonProps) {
  const { t, message } = useTranslations();
  const router = useRouter();
  const [state, setState] = useState<MutationState>({ kind: "idle" });
  const unavailable = disabled || !csrf || state.kind === "saving" || state.kind === "success";

  async function mutate() {
    if (unavailable || !csrf || !window.confirm(confirmMessage)) return;
    setState({ kind: "saving" });
    try {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" },
        body: new URLSearchParams({ csrf }),
      });
      if (response.redirected) {
        router.push(safeRedirectTarget(response.url));
        return;
      }
      if (response.status === 401) {
        router.push(`/login?next=${encodeURIComponent(window.location.pathname)}`);
        return;
      }
      if (!response.ok) {
        setState({ kind: "error", message: await responseMessage(response) });
        return;
      }
      setState({ kind: "success" });
      onSuccess();
    } catch {
      setState({ kind: "error", message: { key: "CodeLocal.Cloud could not be reached." } });
    }
  }

  return (
    <div className={styles.resourceMutation}>
      <button className={styles.resourceDanger} type="button" disabled={unavailable} onClick={mutate}>
        {state.kind === "saving" ? t("Working…") : state.kind === "success" ? t("Updated") : label}
      </button>
      {state.kind === "error" && state.message && <small role="status">{"key" in state.message ? t(state.message.key, state.message.values) : message(state.message.text)}</small>}
    </div>
  );
}
