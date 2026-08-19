import { isIP } from "node:net";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

function isRailwayRuntime() {
  return Boolean(
    process.env.RAILWAY_ENVIRONMENT ||
    process.env.RAILWAY_ENVIRONMENT_ID ||
    process.env.RAILWAY_PROJECT_ID,
  );
}

export function proxy(request: NextRequest) {
  const headers = new Headers(request.headers);

  // Railway's public edge injects X-Real-IP with the remote client IP. When
  // Next proxies an authenticated browser request to the Go service over the
  // private network, collapse the forwarded chain to that edge-owned value so
  // Go never evaluates a client-supplied X-Forwarded-For entry as the browser.
  if (isRailwayRuntime()) {
    const edgeIP = (headers.get("x-real-ip") || "").trim();
    if (isIP(edgeIP)) {
      headers.set("x-real-ip", edgeIP);
      headers.set("x-forwarded-for", edgeIP);
    } else {
      headers.delete("x-real-ip");
      headers.delete("x-forwarded-for");
    }
  }

  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
