import { poolAdminJSON } from "@/lib/server/pool-api";
import { sameOriginMutation } from "@/lib/server/request-security";
import { hasPoolSession } from "@/lib/server/session";

const validModelID = /^[A-Za-z0-9._:-]{1,160}$/;
type RouteContext = { params: Promise<{ id: string }> };

export async function POST(request: Request, context: RouteContext) {
  if (!sameOriginMutation(request)) return Response.json({ error: "invalid_origin" }, { status: 403 });
  if (!(await hasPoolSession())) return Response.json({ error: "unauthorized" }, { status: 401 });
  const { id } = await context.params;
  if (!validModelID.test(id)) return Response.json({ error: "invalid_model" }, { status: 400 });
  try {
    const result = await poolAdminJSON(`/api/models/${id}/test`, { method: "POST" });
    return Response.json(result.data, { status: result.status, headers: { "cache-control": "no-store" } });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}
