import { createPoolSession, verifyPoolPassword } from "@/lib/server/session";
import { isRecord, readJSONBody, sameOriginMutation } from "@/lib/server/request-security";

export async function POST(request: Request) {
  if (!sameOriginMutation(request)) return Response.json({ error: "invalid_origin" }, { status: 403 });
  let body: unknown;
  try { body = await readJSONBody(request, 16 * 1024); } catch { return Response.json({ error: "invalid_request" }, { status: 400 }); }
  const password = isRecord(body) && typeof body.password === "string" ? body.password : "";
  if (!password || !verifyPoolPassword(password)) return Response.json({ error: "invalid_credentials" }, { status: 401 });
  await createPoolSession();
  return Response.json({ ok: true }, { headers: { "cache-control": "no-store" } });
}
