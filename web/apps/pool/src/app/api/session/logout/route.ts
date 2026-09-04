import { deletePoolSession } from "@/lib/server/session";
import { sameOriginMutation } from "@/lib/server/request-security";

export async function POST(request: Request) {
  if (!sameOriginMutation(request)) return Response.json({ error: "invalid_origin" }, { status: 403 });
  await deletePoolSession();
  return Response.json({ ok: true }, { headers: { "cache-control": "no-store" } });
}
