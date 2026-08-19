"use client";

import { useSyncExternalStore } from "react";
import { CopyButton } from "../copy-button";
import surface from "../dashboard-surfaces.module.css";

const subscribe = () => () => undefined;

export function MCPEndpoint() {
  const endpoint = useSyncExternalStore(
    subscribe,
    () => `${window.location.origin}/mcp`,
    () => "/mcp",
  );
  return (
    <div className={surface.codeRow}>
      <div className={surface.code}>{endpoint}</div>
      <CopyButton value={endpoint} label="Copy URL" />
    </div>
  );
}
