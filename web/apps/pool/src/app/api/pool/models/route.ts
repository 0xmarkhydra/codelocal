import { poolAdminJSON } from "@/lib/server/pool-api";
import { hasPoolSession } from "@/lib/server/session";

export async function GET() {
  if (!(await hasPoolSession())) return Response.json({ error: "unauthorized" }, { status: 401 });
  try {
    const result = await poolAdminJSON("/api/models");
    return Response.json(result.data, { status: result.status, headers: { "cache-control": "no-store" } });
  } catch {
    return Response.json({ error: "pool_unavailable" }, { status: 502 });
  }
}
