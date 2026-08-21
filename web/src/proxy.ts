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

function backendURL(request: NextRequest) {
  const raw = (process.env.CODELOCAL_BACKEND_URL || "http://127.0.0.1:3333").trim();
  const backend = new URL(raw);
  if (backend.protocol !== "http:" && backend.protocol !== "https:") {
    throw new Error("CODELOCAL_BACKEND_URL must use http or https");
  }
  backend.pathname = request.nextUrl.pathname;
  backend.search = request.nextUrl.search;
  backend.hash = "";
  return backend;
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

  // The direct Next DEV canary must remain a usable browser surface. GET/HEAD
  // presentation stays in Next, while every trust-bearing form mutation is
  // rewritten to the Go authority. This preserves Go session/CSRF/rate-limit
  // semantics and lets the returned host-only cookies belong to the canary
  // origin instead of producing a UI that appears signed in but only gets 401s.
  if (isGoMutation(request)) {
    return NextResponse.rewrite(backendURL(request), { request: { headers } });
  }

  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
