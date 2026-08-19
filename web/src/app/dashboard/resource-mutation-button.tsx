"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import styles from "./dashboard.module.css";

type MutationState = { kind: "idle" | "saving" | "success" | "error"; message?: string };

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

async function responseMessage(response: Response) {
  try {
    const payload = await response.json() as { message?: unknown; error?: unknown };
    if (typeof payload.message === "string" && payload.message.length <= 240) return payload.message;
    switch (payload.error) {
      case "invalid_csrf": return "Security token expired. Reload the page and try again.";
      case "workspace_runtime_offline": return "Start CodeLocal on that machine before removing workspace access.";
      case "device_not_found": return "This device is no longer available to revoke.";
      case "workspace_not_found": return "This workspace is no longer authorized.";
      case "device_already_revoked": return "This device has already been revoked.";
      default: return `The Go backend rejected this change (${response.status}).`;
    }
  } catch {
    return `The Go backend rejected this change (${response.status}).`;
  }
}

export function ResourceMutationButton({ endpoint, csrf, label, confirmMessage, disabled, onSuccess }: ResourceMutationButtonProps) {
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
      setState({ kind: "success", message: "Updated" });
      onSuccess();
    } catch {
      setState({ kind: "error", message: "The Go backend could not be reached." });
    }
  }

  return (
    <div className={styles.resourceMutation}>
      <button className={styles.resourceDanger} type="button" disabled={unavailable} onClick={mutate}>
        {state.kind === "saving" ? "Working…" : state.kind === "success" ? "Updated" : label}
      </button>
      {state.kind === "error" && state.message && <small role="status">{state.message}</small>}
    </div>
  );
}
