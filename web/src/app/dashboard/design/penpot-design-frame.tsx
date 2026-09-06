"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isWorkspacesResource } from "@/lib/contracts/resources";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./design.module.css";

const PENPOT_TOKEN_MESSAGE = "codelocal:penpot-mcp-token";
const MAX_AUTO_CONNECTIONS = 10;

type PenpotTokenMessage = {
  type: typeof PENPOT_TOKEN_MESSAGE;
  token: string;
};

function isPenpotTokenMessage(value: unknown): value is PenpotTokenMessage {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  return record.type === PENPOT_TOKEN_MESSAGE &&
    typeof record.token === "string" &&
    record.token.trim().length >= 20 &&
    record.token.length <= 8192 &&
    !/[\r\n\0]/.test(record.token);
}

export function PenpotDesignFrame({ designUrl }: { designUrl: string }) {
  const frameRef = useRef<HTMLIFrameElement>(null);
  const tokenRef = useRef("");
  const connectedRef = useRef(new Set<string>());
  const [tokenRevision, setTokenRevision] = useState(0);
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const designOrigin = useMemo(() => {
    try {
      return new URL(designUrl).origin;
    } catch {
      return "";
    }
  }, [designUrl]);

  useEffect(() => {
    function onMessage(event: MessageEvent) {
      if (!designOrigin || event.origin !== designOrigin) return;
      if (!frameRef.current || event.source !== frameRef.current.contentWindow) return;
      if (!isPenpotTokenMessage(event.data)) return;
      const token = event.data.token.trim();
      if (token === tokenRef.current) return;
      tokenRef.current = token;
      connectedRef.current.clear();
      setTokenRevision((value) => value + 1);
    }

    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [designOrigin]);

  useEffect(() => {
    const token = tokenRef.current;
    if (!token || account.state.kind !== "ready" || workspaces.state.kind !== "ready") return;

    const csrf = account.state.value.csrf;
    const targets = workspaces.state.value.items
      .filter((workspace) => workspace.runtimeOnline && workspace.status !== "offline")
      .slice(0, MAX_AUTO_CONNECTIONS);
    if (!targets.length) return;

    const controller = new AbortController();
    void Promise.allSettled(targets.map(async (workspace) => {
      const key = `${workspace.deviceId}:${workspace.workspaceId}`;
      if (connectedRef.current.has(key)) return;

      const response = await fetch("/api/v1/plugins/penpot/connections", {
        method: "POST",
        credentials: "same-origin",
        cache: "no-store",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
        },
        body: JSON.stringify({
          deviceId: workspace.deviceId,
          workspaceId: workspace.workspaceId,
          endpoint: `${designOrigin}/mcp/stream`,
          bearerToken: token,
        }),
        signal: controller.signal,
      });
      if (response.ok) connectedRef.current.add(key);
    }));

    return () => controller.abort();
  }, [account.state, designOrigin, tokenRevision, workspaces.state]);

  return (
    <iframe
      ref={frameRef}
      allow="clipboard-read; clipboard-write"
      className={styles.designFrame}
      referrerPolicy="strict-origin-when-cross-origin"
      src={designUrl}
      title="CodeLocal Penpot Design Workspace"
    />
  );
}
