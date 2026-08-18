import "server-only";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:3333";

function normalizedBackendURL() {
  const raw = (process.env.CODELOCAL_BACKEND_URL || DEFAULT_BACKEND_URL).trim();
  const url = new URL(raw);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("CODELOCAL_BACKEND_URL must use http or https");
  }
  url.pathname = url.pathname.replace(/\/$/, "");
  url.search = "";
  url.hash = "";
  return url;
}

export function backendURL(pathname: string) {
  const base = normalizedBackendURL();
  const safePath = pathname.startsWith("/") ? pathname : `/${pathname}`;
  return new URL(safePath, `${base.toString().replace(/\/$/, "")}/`);
}

export async function getBackendHealth(signal?: AbortSignal) {
  const response = await fetch(backendURL("/health"), {
    method: "GET",
    cache: "no-store",
    signal,
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(`CodeLocal backend health request failed with ${response.status}`);
  }

  return response.json() as Promise<unknown>;
}

// Do not add a generic authenticated proxy here. Browser/session migration must
// preserve the Go backend's existing cookie, CSRF, rate-limit and authorization
// semantics route by route before any production cutover.
