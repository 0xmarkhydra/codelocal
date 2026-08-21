import { isIP } from "node:net";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const GO_MUTATION_PATHS = new Set([
  "/login",
  "/signup",
  "/signup/verify",
  "/forgot-password",
  "/reset-password",
  "/logout",
  "/account/password",
  "/authorize",
  "/pair/approve",
]);

function isRailwayRuntime() {
  return Boolean(
    process.env.RAILWAY_ENVIRONMENT ||
    process.env.RAILWAY_ENVIRONMENT_ID ||
    process.env.RAILWAY_PROJECT_ID,
  );
}

function isGoMutation(request: NextRequest) {
  return request.method !== "GET" && request.method !== "HEAD" && GO_MUTATION_PATHS.has(request.nextUrl.pathname);
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

  // Never trust a client-supplied routing marker. Proxy runs before
  // next.config rewrites, so mark only the explicit trust-bearing mutations
  // that should be transported to Go while GET/HEAD presentation stays in Next.
  headers.delete("x-codelocal-go-mutation");
  if (isGoMutation(request)) {
    headers.set("x-codelocal-go-mutation", "1");
  }

  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};