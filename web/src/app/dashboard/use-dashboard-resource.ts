"use client";

import { useCallback, useEffect, useState } from "react";

export type DashboardResourceState<T> =
  | { kind: "loading" }
  | { kind: "ready"; value: T }
  | { kind: "unauthenticated" }
  | { kind: "error"; message: string };

export function useDashboardResource<T>(url: string, validate: (value: unknown) => value is T) {
  const [state, setState] = useState<DashboardResourceState<T>>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        const response = await fetch(url, {
          method: "GET",
          credentials: "same-origin",
          cache: "no-store",
          headers: { Accept: "application/json" },
          signal: controller.signal,
        });
        if (response.status === 401) {
          setState({ kind: "unauthenticated" });
          return;
        }
        if (!response.ok) {
          setState({ kind: "error", message: `Backend returned ${response.status}.` });
          return;
        }
        const body: unknown = await response.json();
        if (!validate(body)) {
          setState({ kind: "error", message: "Backend response did not match the expected contract." });
          return;
        }
        setState({ kind: "ready", value: body });
      } catch (error) {
        if (controller.signal.aborted) return;
        setState({
          kind: "error",
          message: error instanceof Error ? error.message : "Unable to reach the CodeLocal.Cloud backend.",
        });
      }
    }

    void load();
    return () => controller.abort();
  }, [attempt, url, validate]);

  const retry = useCallback(() => {
    setState({ kind: "loading" });
    setAttempt((value) => value + 1);
  }, []);

  return { state, retry };
}
