import { createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import express, { type NextFunction, type Request, type Response } from "express";
import { cloudStore } from "./cloud-store.js";
import { getWebIdentity, verifyCsrf } from "./saas-auth.js";
import { authPage, escapeHtml } from "./web-ui.js";
import { rateLimit } from "./rate-limit.js";

const BASE_URL = (process.env.PUBLIC_BASE_URL ?? "").replace(/\/$/, "");
const MCP_AUTH_SECRET = process.env.MCP_AUTH_SECRET ?? "";
if (!BASE_URL || !MCP_AUTH_SECRET) throw new Error("Missing PUBLIC_BASE_URL or MCP_AUTH_SECRET");

export const MCP_RESOURCE = `${BASE_URL}/mcp`;
const ACCESS_TTL_SECONDS = 60 * 60;
const REFRESH_TTL_SECONDS = 30 * 24 * 60 * 60;
const CODE_TTL_MS = 5 * 60 * 1000;
const SCOPE = "mcp:tools offline_access";

type TokenPayload = {
  typ: "access" | "refresh";
  sub: string;
  client_id: string;
  resource: string;
  scope: string;
  iat: number;
  exp: number;
  jti: string;
};

function b64url(value: Buffer | string) { return Buffer.from(value).toString("base64url"); }
function signToken(payload: TokenPayload) {
  const encoded = b64url(JSON.stringify(payload));
  const sig = createHmac("sha256", MCP_AUTH_SECRET).update(encoded).digest("base64url");
  return `${encoded}.${sig}`;
}
function verifyToken(token: string, expectedType: TokenPayload["typ"]): TokenPayload {
  const [encoded, signature] = token.split(".");
  if (!encoded || !signature) throw new Error("Malformed token");
  const expected = createHmac("sha256", MCP_AUTH_SECRET).update(encoded).digest();
  const actual = Buffer.from(signature, "base64url");
  if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) throw new Error("Invalid token signature");
  const payload = JSON.parse(Buffer.from(encoded, "base64url").toString("utf8")) as TokenPayload;
  const now = Math.floor(Date.now() / 1000);
  if (payload.typ !== expectedType || payload.exp <= now || payload.resource !== MCP_RESOURCE || !payload.sub) throw new Error("Expired or invalid token");
  return payload;
}
function issueTokens(userId: string, clientId: string, resource: string, scope = SCOPE) {
  const now = Math.floor(Date.now() / 1000);
  return {
    access_token: signToken({ typ: "access", sub: userId, client_id: clientId, resource, scope, iat: now, exp: now + ACCESS_TTL_SECONDS, jti: randomBytes(16).toString("hex") }),
    refresh_token: signToken({ typ: "refresh", sub: userId, client_id: clientId, resource, scope, iat: now, exp: now + REFRESH_TTL_SECONDS, jti: randomBytes(16).toString("hex") }),
    token_type: "Bearer",
    expires_in: ACCESS_TTL_SECONDS,
    scope,
  };
}
function validRedirectUri(value: string) {
  try {
    const url = new URL(value.trim());
    if (url.username || url.password || url.hash) return false;
    const host = url.hostname.replace(/^\[/, "").replace(/\]$/, "");
    if (url.protocol === "https:") return !!host;
    if (url.protocol === "http:") return ["localhost", "127.0.0.1", "::1"].includes(host.toLowerCase());
    if (["javascript:", "data:", "file:", "vbscript:"].includes(url.protocol.toLowerCase())) return false;
    return !!(host || url.pathname);
  } catch { return false; }
}
function htmlEscape(value: string) { return escapeHtml(value); }
function parseScope(scope?: string) {
  const requested = new Set((scope ?? SCOPE).split(/\s+/).filter(Boolean));
  requested.add("mcp:tools"); requested.add("offline_access");
  return [...requested].join(" ");
}
function loginNext(req: Request) {
  const path = req.originalUrl.startsWith("/") ? req.originalUrl : "/authorize";
  return `/login?next=${encodeURIComponent(path)}`;
}

export const oauthRouter = express.Router();
oauthRouter.use(express.urlencoded({ extended: false, limit: "64kb" }));
const oauthRegisterRateLimit = rateLimit({ scope: "oauth-register-ip", limit: 30, windowSeconds: 60 });
const oauthTokenIpRateLimit = rateLimit({ scope: "oauth-token-ip", limit: 120, windowSeconds: 60 });
const oauthTokenClientRateLimit = rateLimit({
  scope: "oauth-token-client",
  limit: 60,
  windowSeconds: 60,
  subject: (req) => String(req.body?.client_id ?? "unknown").slice(0, 160) || "unknown",
});
oauthRouter.get("/.well-known/oauth-protected-resource", (_req, res) => res.json({ resource: MCP_RESOURCE, authorization_servers: [BASE_URL], scopes_supported: ["mcp:tools", "offline_access"], bearer_methods_supported: ["header"] }));
oauthRouter.get("/.well-known/oauth-authorization-server", (_req, res) => res.json({ issuer: BASE_URL, authorization_endpoint: `${BASE_URL}/authorize`, token_endpoint: `${BASE_URL}/token`, registration_endpoint: `${BASE_URL}/register`, response_types_supported: ["code"], grant_types_supported: ["authorization_code", "refresh_token"], code_challenge_methods_supported: ["S256"], token_endpoint_auth_methods_supported: ["none"], scopes_supported: ["mcp:tools", "offline_access"] }));

oauthRouter.post("/register", express.json({ limit: "64kb" }), oauthRegisterRateLimit, async (req, res) => {
  const rawRedirectUris = req.body?.redirect_uris;
  const raw = Array.isArray(rawRedirectUris)
    ? rawRedirectUris
    : typeof rawRedirectUris === "string"
      ? [rawRedirectUris]
      : typeof req.body?.redirect_uri === "string"
        ? [req.body.redirect_uri]
        : [];
  const redirectUris: string[] = raw.filter((value: unknown): value is string => typeof value === "string" && value.trim().length > 0);
  if (!redirectUris.length) { res.status(400).json({ error: "invalid_redirect_uri", error_description: "At least one redirect URI is required." }); return; }
  const invalidIndex = redirectUris.findIndex((redirectUri) => !validRedirectUri(redirectUri));
  if (invalidIndex >= 0) { res.status(400).json({ error: "invalid_redirect_uri", error_description: `Redirect URI at index ${invalidIndex} is not an absolute OAuth callback URI.` }); return; }
  const clientId = `codelocal_${randomBytes(24).toString("base64url")}`;
  const client = await cloudStore.createOAuthClient({ clientId, redirectUris, clientName: typeof req.body?.client_name === "string" ? req.body.client_name.slice(0, 160) : undefined });
  res.status(201).json({ client_id: client.clientId, client_name: client.clientName ?? "MCP client", redirect_uris: client.redirectUris, grant_types: ["authorization_code", "refresh_token"], response_types: ["code"], token_endpoint_auth_method: "none" });
});

oauthRouter.get("/authorize", async (req, res) => {
  const clientId = String(req.query.client_id ?? "");
  const redirectUri = String(req.query.redirect_uri ?? "");
  const responseType = String(req.query.response_type ?? "");
  const codeChallenge = String(req.query.code_challenge ?? "");
  const codeChallengeMethod = String(req.query.code_challenge_method ?? "");
  const resource = String(req.query.resource ?? "");
  const scope = parseScope(typeof req.query.scope === "string" ? req.query.scope : undefined);
  const state = typeof req.query.state === "string" ? req.query.state : "";
  const client = await cloudStore.oauthClient(clientId);
  if (!client || !client.redirectUris.includes(redirectUri) || responseType !== "code" || !codeChallenge || codeChallengeMethod !== "S256" || resource !== MCP_RESOURCE) {
    res.status(400).send("Invalid OAuth authorization request."); return;
  }
  const me = await getWebIdentity(req);
  if (!me) { res.redirect(302, loginNext(req)); return; }
  const hidden = { client_id: clientId, redirect_uri: redirectUri, code_challenge: codeChallenge, resource, scope, state, csrf: me.csrf };
  const hiddenInputs = Object.entries(hidden).map(([key, value]) => `<input type="hidden" name="${key}" value="${htmlEscape(value)}">`).join("\n");
  res.type("html").send(authPage({
    title: "Connect ChatGPT",
    subtitle: `Signed in as ${me.user.email}. ChatGPT will only see devices and workspaces belonging to this CodeLocal account.`,
    body: `<div class="card" style="box-shadow:none;padding:16px;margin-bottom:16px"><div class="label">MCP client</div><div class="row-title" style="margin-top:4px">${htmlEscape(client.clientName ?? "ChatGPT")}</div><div class="row-meta mono">${htmlEscape(MCP_RESOURCE)}</div></div><form class="form" method="post" action="/authorize">${hiddenInputs}<button class="btn primary" type="submit">Authorize ChatGPT</button><a class="btn" href="/dashboard">Cancel</a></form>`,
  }));
});

oauthRouter.post("/authorize", async (req, res) => {
  const me = await getWebIdentity(req);
  if (!me) { res.status(401).send("Sign in to CodeLocal and restart the ChatGPT connection flow."); return; }
  if (!verifyCsrf(req)) { res.status(403).send("Invalid security token. Restart the authorization flow."); return; }
  const clientId = String(req.body.client_id ?? "");
  const redirectUri = String(req.body.redirect_uri ?? "");
  const codeChallenge = String(req.body.code_challenge ?? "");
  const resource = String(req.body.resource ?? "");
  const scope = parseScope(String(req.body.scope ?? SCOPE));
  const state = String(req.body.state ?? "");
  const client = await cloudStore.oauthClient(clientId);
  if (!client || !client.redirectUris.includes(redirectUri) || !codeChallenge || resource !== MCP_RESOURCE) { res.status(400).send("Invalid OAuth authorization request."); return; }
  const code = randomBytes(32).toString("base64url");
  await cloudStore.putOAuthCode({ code, userId: me.user.id, clientId, redirectUri, codeChallenge, resource, scope, expiresAt: Date.now() + CODE_TTL_MS });
  await cloudStore.audit(me.user.id, "oauth.authorized", { clientId, clientName: client.clientName ?? null });
  const target = new URL(redirectUri);
  target.searchParams.set("code", code);
  if (state) target.searchParams.set("state", state);
  res.redirect(302, target.toString());
});

oauthRouter.post("/token", oauthTokenIpRateLimit, oauthTokenClientRateLimit, async (req, res) => {
  res.setHeader("Cache-Control", "no-store"); res.setHeader("Pragma", "no-cache");
  const grantType = String(req.body.grant_type ?? "");
  const clientId = String(req.body.client_id ?? "");
  const resource = String(req.body.resource ?? "");
  try {
    if (grantType === "authorization_code") {
      const code = String(req.body.code ?? "");
      const verifier = String(req.body.code_verifier ?? "");
      const redirectUri = String(req.body.redirect_uri ?? "");
      const record = await cloudStore.consumeOAuthCode(code);
      if (!record || record.expiresAt < Date.now() || record.clientId !== clientId || record.redirectUri !== redirectUri || record.resource !== resource) throw new Error("invalid_grant");
      const challenge = createHash("sha256").update(verifier).digest("base64url");
      if (!verifier || challenge !== record.codeChallenge) throw new Error("invalid_grant");
      res.json(issueTokens(record.userId, clientId, resource, record.scope)); return;
    }
    if (grantType === "refresh_token") {
      const payload = verifyToken(String(req.body.refresh_token ?? ""), "refresh");
      if (payload.client_id !== clientId || payload.resource !== resource) throw new Error("invalid_grant");
      res.json(issueTokens(payload.sub, clientId, resource, payload.scope)); return;
    }
    res.status(400).json({ error: "unsupported_grant_type" });
  } catch { res.status(400).json({ error: "invalid_grant" }); }
});

function unauthorized(res: Response) {
  res.setHeader("WWW-Authenticate", `Bearer realm="codelocal", resource_metadata="${BASE_URL}/.well-known/oauth-protected-resource"`);
  res.status(401).json({ error: "unauthorized" });
}
export function requireMcpAuth(req: Request, res: Response, next: NextFunction) {
  const header = req.headers.authorization;
  if (!header?.startsWith("Bearer ")) { unauthorized(res); return; }
  try {
    const payload = verifyToken(header.slice(7), "access");
    if (!payload.scope.split(/\s+/).includes("mcp:tools")) { res.status(403).json({ error: "insufficient_scope" }); return; }
    res.locals.oauth = payload;
    next();
  } catch { unauthorized(res); }
}
