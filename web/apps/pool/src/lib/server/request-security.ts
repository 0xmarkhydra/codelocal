import "server-only";

function firstHeaderValue(value: string | null): string {
  return (value || "").split(",", 1)[0]?.trim() || "";
}

function expectedOrigin(request: Request): string {
  const url = new URL(request.url);
  const forwardedProto = firstHeaderValue(request.headers.get("x-forwarded-proto"));
  const forwardedHost = firstHeaderValue(request.headers.get("x-forwarded-host"));
  const host = forwardedHost || firstHeaderValue(request.headers.get("host")) || url.host;
  const proto = forwardedProto || url.protocol.replace(/:$/, "");
  if ((proto !== "http" && proto !== "https") || !host || /[\s/\\]/.test(host)) return url.origin;
  return `${proto}://${host}`;
}

export function sameOriginMutation(request: Request): boolean {
  const fetchSite = firstHeaderValue(request.headers.get("sec-fetch-site")).toLowerCase();
  if (fetchSite === "cross-site") return false;
  const origin = firstHeaderValue(request.headers.get("origin"));
  if (!origin) return true;
  try {
    return new URL(origin).origin === expectedOrigin(request);
  } catch {
    return false;
  }
}

export async function readJSONBody(request: Request, maxBytes = 128 * 1024): Promise<unknown> {
  const declared = Number(request.headers.get("content-length") || "0");
  if (Number.isFinite(declared) && declared > maxBytes) throw new Error("request_too_large");
  const text = await request.text();
  if (Buffer.byteLength(text, "utf8") > maxBytes) throw new Error("request_too_large");
  if (!text.trim()) throw new Error("invalid_json");
  try {
    return JSON.parse(text);
  } catch {
    throw new Error("invalid_json");
  }
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
