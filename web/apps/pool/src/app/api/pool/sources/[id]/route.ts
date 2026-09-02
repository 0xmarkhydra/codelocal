import { poolAdminJSON } from "@/lib/server/pool-api";
import { isRecord, readJSONBody, sameOriginMutation } from "@/lib/server/request-security";
import { hasPoolSession } from "@/lib/server/session";

const validID = /^[A-Za-z0-9_-]{1,96}$/;

type RouteContext = { params: Promise<{ id: string }> };

export async function PATCH(request: Request, context: RouteContext) {
  if (!sameOriginMutation(request)) return Response.json({ error: "invalid_origin" }, { status: 403 });
  if (!(await hasPoolSession())) return Response.json({ error: "unauthorized" }, { status: 401 });
  const { id } = await context.params;
  if (!validID.test(id)) return Response.json({ error: "invalid_source" }, { status: 400 });
  let raw: unknown;
  try { raw = await readJSONBody(request, 16 * 1024); } catch { return Response.json({ error: "invalid_request" }, { status: 400 }); }
  if (!isRecord(raw) || typeof raw.enabled !== "boolean") return Response.json({ error: "invalid_source_update" }, { status: 400 });
  try {
    const result = await poolAdminJSON(`/api/sources/${id}`, { method: "PATCH", body: JSON.stringify({ enabled: raw.enabled }) });
    return Response.json(result.data, { status: result.status, headers: { "cache-control": "no-store" } });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}

export async function DELETE(request: Request, context: RouteContext) {
  if (!sameOriginMutation(request)) return Response.json({ error: "invalid_origin" }, { status: 403 });
  if (!(await hasPoolSession())) return Response.json({ error: "unauthorized" }, { status: 401 });
  const { id } = await context.params;
  if (!validID.test(id)) return Response.json({ error: "invalid_source" }, { status: 400 });
  try {
    const result = await poolAdminJSON(`/api/sources/${id}`, { method: "DELETE" });
    return Response.json(result.data, { status: result.status, headers: { "cache-control": "no-store" } });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}
