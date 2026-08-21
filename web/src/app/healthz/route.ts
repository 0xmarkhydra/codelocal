import { NextResponse } from "next/server";

import { getBackendHealth } from "@/lib/server/backend";

const BACKEND_HEALTH_TIMEOUT_MS = 2_500;

export async function GET() {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), BACKEND_HEALTH_TIMEOUT_MS);

  try {
    await getBackendHealth(controller.signal);
    return NextResponse.json(
      { ok: true, service: "codelocal-web", backend: "ready" },
      { status: 200, headers: { "Cache-Control": "no-store" } },
    );
  } catch {
    return NextResponse.json(
      { ok: false, service: "codelocal-web", backend: "unavailable" },
      { status: 503, headers: { "Cache-Control": "no-store" } },
    );
  } finally {
    clearTimeout(timeout);
  }
}
