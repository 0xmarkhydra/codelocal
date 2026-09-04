import "server-only";

const MAX_ADMIN_RESPONSE_BYTES = 4 * 1024 * 1024;
const MAX_CLIENT_REQUEST_BYTES = 16 * 1024 * 1024;
const CLIENT_PATHS = new Set(["/v1/models", "/v1/chat/completions", "/v1/responses"]);

function poolOrigin(): string {
  const raw = (process.env.POOL_API_BASE_URL || "").trim();
  if (!raw) throw new Error("POOL_API_BASE_URL is required");
  const parsed = new URL(raw);
  if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.username || parsed.password) {
    throw new Error("POOL_API_BASE_URL must be an http(s) origin without embedded credentials");
  }
  parsed.pathname = parsed.pathname.replace(/\/v1\/?$/i, "").replace(/\/$/, "");
  parsed.search = "";
  parsed.hash = "";
  return parsed.toString().replace(/\/$/, "");
}

function adminToken(): string {
  const value = (process.env.POOL_ADMIN_TOKEN || "").trim();
  if (!value) throw new Error("POOL_ADMIN_TOKEN is required");
  return value;
}

function safeAdminPath(path: string): boolean {
  return /^\/api\/(models(?:\/[A-Za-z0-9._:-]{1,160}\/test)?|sources(?:\/[A-Za-z0-9_-]{1,96})?)$/.test(path);
}

export type PoolAdminResult = { status: number; data: unknown };

export async function poolAdminJSON(path: string, init: RequestInit = {}): Promise<PoolAdminResult> {
  if (!safeAdminPath(path)) throw new Error("invalid Pool admin path");
  const response = await fetch(`${poolOrigin()}${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${adminToken()}`,
      ...(init.body ? { "Content-Type": "application/json" } : {}),
    },
    signal: AbortSignal.timeout(path.endsWith("/test") ? 95_000 : 12_000),
  });
  const raw = await response.text();
  if (Buffer.byteLength(raw, "utf8") > MAX_ADMIN_RESPONSE_BYTES) throw new Error("Pool admin response is too large");
  let data: unknown = {};
  if (raw.trim()) {
    try { data = JSON.parse(raw); } catch { data = { error: "invalid_pool_response" }; }
  }
  return { status: response.status, data };
}

function clientHeaders(request: Request): Headers {
  const headers = new Headers();
  for (const name of ["authorization", "accept", "content-type", "x-request-id"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  return headers;
}

function gatewayResponseHeaders(upstream: Response): Headers {
  const headers = new Headers();
  for (const [name, value] of upstream.headers) {
    const lower = name.toLowerCase();
    if (lower === "content-type" || lower === "cache-control" || lower === "retry-after" || lower === "x-request-id" || lower.startsWith("x-ratelimit-")) headers.set(name, value);
  }
  headers.set("X-Content-Type-Options", "nosniff");
  return headers;
}

export async function poolClientProxy(request: Request, path: string): Promise<Response> {
  if (!CLIENT_PATHS.has(path)) return Response.json({ error: "unsupported_pool_path" }, { status: 404 });
  if (request.method !== "GET" && request.method !== "POST") return Response.json({ error: "method_not_allowed" }, { status: 405 });

  let body: ArrayBuffer | undefined;
  if (request.method === "POST") {
    body = await request.arrayBuffer();
    if (body.byteLength > MAX_CLIENT_REQUEST_BYTES) return Response.json({ error: "request_too_large" }, { status: 413 });
  }

  try {
    const upstream = await fetch(`${poolOrigin()}${path}`, {
      method: request.method,
      headers: clientHeaders(request),
      body,
      cache: "no-store",
      signal: request.signal,
    });
    return new Response(upstream.body, {
      status: upstream.status,
      statusText: upstream.statusText,
      headers: gatewayResponseHeaders(upstream),
    });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}

export async function poolHealth(): Promise<Response> {
  try {
    const upstream = await fetch(`${poolOrigin()}/health`, { cache: "no-store", signal: AbortSignal.timeout(5_000) });
    const raw = await upstream.text();
    return new Response(raw, { status: upstream.status, headers: { "content-type": upstream.headers.get("content-type") || "application/json", "cache-control": "no-store" } });
  } catch {
    return Response.json({ ok: false, error: "pool_unavailable" }, { status: 503 });
  }
}
