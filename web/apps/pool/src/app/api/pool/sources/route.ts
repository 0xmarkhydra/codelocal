import { poolAdminJSON } from "@/lib/server/pool-api";
import { isRecord, readJSONBody, sameOriginMutation } from "@/lib/server/request-security";
import { hasPoolSession } from "@/lib/server/session";

export async function GET() {
  if (!(await hasPoolSession())) return Response.json({ error: "unauthorized" }, { status: 401 });
  try {
    const result = await poolAdminJSON("/api/sources");
    return Response.json(result.data, { status: result.status, headers: { "cache-control": "no-store" } });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}

export async function POST(request: Request) {
  if (!sameOriginMutation(request)) return Response.json({ error: "invalid_origin" }, { status: 403 });
  if (!(await hasPoolSession())) return Response.json({ error: "unauthorized" }, { status: 401 });
  let raw: unknown;
  try { raw = await readJSONBody(request); } catch { return Response.json({ error: "invalid_request" }, { status: 400 }); }
  if (!isRecord(raw)) return Response.json({ error: "invalid_request" }, { status: 400 });
  const input = {
    name: typeof raw.name === "string" ? raw.name.trim() : "",
    kind: typeof raw.kind === "string" ? raw.kind.trim() : "openai-compatible",
    baseUrl: typeof raw.baseUrl === "string" ? raw.baseUrl.trim() : "",
    apiKey: typeof raw.apiKey === "string" ? raw.apiKey.trim() : "",
    priority: typeof raw.priority === "number" && Number.isInteger(raw.priority) ? raw.priority : 50,
  };
  if (!input.name || !input.baseUrl || !input.apiKey || input.priority < 0 || input.priority > 10000) return Response.json({ error: "invalid_source" }, { status: 400 });
  try {
    const result = await poolAdminJSON("/api/sources", { method: "POST", body: JSON.stringify(input) });
    return Response.json(result.data, { status: result.status, headers: { "cache-control": "no-store" } });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}
