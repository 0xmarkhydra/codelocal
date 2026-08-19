"use client";

import { useEffect, useState } from "react";

type CSRFResponse = { csrf?: unknown };

export function CSRFFormToken() {
  const [csrf, setCSRF] = useState("");

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v1/auth/csrf", { credentials: "same-origin", cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error("csrf_unavailable");
        return response.json() as Promise<CSRFResponse>;
      })
      .then((payload) => {
        if (!cancelled && typeof payload.csrf === "string") setCSRF(payload.csrf);
      })
      .catch(() => {
        if (!cancelled) setCSRF("");
      });
    return () => { cancelled = true; };
  }, []);

  return <input type="hidden" name="csrf" value={csrf} />;
}
