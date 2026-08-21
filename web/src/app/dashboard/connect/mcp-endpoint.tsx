"use client";

import { useSyncExternalStore } from "react";
import { CopyButton } from "../copy-button";
import visual from "../visual-dashboard.module.css";

const subscribe = () => () => undefined;

export function MCPEndpoint() {
  const endpoint = useSyncExternalStore(subscribe, () => `${window.location.origin}/mcp`, () => "/mcp");
  return <div className={visual.endpoint}><span className={visual.endpointDot} /><code>{endpoint}</code><CopyButton value={endpoint} label="Copy" /></div>;
}
