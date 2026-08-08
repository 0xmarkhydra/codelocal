import { createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import express, { type NextFunction, type Request, type Response } from "express";
import path from "node:path";
import { DEFAULT_STATE_DIR, readJsonFile, writeJsonAtomic } from "./state.js";

const BASE_URL = (process.env.PUBLIC_BASE_URL ?? "").replace(/\/$/, "");
const MCP_USER_PASSWORD = process.env.MCP_USER_PASSWORD ?? "";
const MCP_AUTH_SECRET = process.env.MCP_AUTH_SECRET ?? "";
if (!BASE_URL || !MCP_USER_PASSWORD || !MCP_AUTH_SECRET) throw new Error("Missing PUBLIC_BASE_URL, MCP_USER_PASSWORD or MCP_AUTH_SECRET");

export const MCP_RESOURCE = `${BASE_URL}/mcp`;
const ACCESS_TTL_SECONDS = 60 * 60;
const REFRESH_TTL_SECONDS = 30 * 24 * 60 * 60;
const CODE_TTL_MS = 5 * 60 * 1000;
const SCOPE = "mcp:tools offline_access";
const CLIENT_FILE = process.env.CODELOCAL_OAUTH_CLIENT_FILE ?? path.join(DEFAULT_STATE_DIR, "oauth-clients.json");

type ClientRegistration = { clientId: string; redirectUris: string[]; clientName?: string };
type AuthCode = { clientId: string; redirectUri: string; codeChallenge: string; resource: string; scope: string; expiresAt: number };
type TokenPayload = { typ: "access" | "refresh"; client_id: string; resource: string; scope: string; iat: number; exp: number; jti: string };

const clients = new Map<string, ClientRegistration>();
const authCodes = new Map<string, AuthCode>();
let loaded = false;

async function loadClients() {
  if (loaded) return;
  const saved = await readJsonFile<ClientRegistration[]>(CLIENT_FILE, []);
  for (const client of saved) clients.set(client.clientId, client);
  loaded = true;
}
async function saveClients() { await writeJsonAtomic(CLIENT_FILE, [...clients.values()]); }
function b64url(value: Buffer | string) { return Buffer.from(value).toString("base64url"); }
function signToken(payload: TokenPayload) { const encoded = b64url(JSON.stringify(payload)); const sig = createHmac("sha256", MCP_AUTH_SECRET).update(encoded).digest("base64url"); return `${encoded}.${sig}`; }
function verifyToken(token: string, expectedType: TokenPayload["typ"]): TokenPayload {
  const [encoded, signature] = token.split(".");
  if (!encoded || !signature) throw new Error("Malformed token");
  const expected = createHmac("sha256", MCP_AUTH_SECRET).update(encoded).digest();
  const actual = Buffer.from(signature, "base64url");
  if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) throw new Error("Invalid token signature");
  const payload = JSON.parse(Buffer.from(encoded, "base64url").toString("utf8")) as TokenPayload;
  const now = Math.floor(Date.now() / 1000);
  if (payload.typ !== expectedType || payload.exp <= now || payload.resource !== MCP_RESOURCE) throw new Error("Expired or invalid token");
  return payload;
}
function issueTokens(clientId: string, resource: string, scope = SCOPE) {
  const now = Math.floor(Date.now() / 1000);
  return {
    access_token: signToken({ typ: "access", client_id: clientId, resource, scope, iat: now, exp: now + ACCESS_TTL_SECONDS, jti: randomBytes(16).toString("hex") }),
    refresh_token: signToken({ typ: "refresh", client_id: clientId, resource, scope, iat: now, exp: now + REFRESH_TTL_SECONDS, jti: randomBytes(16).toString("hex") }),
    token_type: "Bearer",
    expires_in: ACCESS_TTL_SECONDS,
    scope,
  };
}
function validRedirectUri(value: string) { try { const url = new URL(value); return url.protocol === "https:" || (url.protocol === "http:" && ["localhost", "127.0.0.1", "::1"].includes(url.hostname)); } catch { return false; } }
function equalSecret(a: string, b: string) { const aa = Buffer.from(a); const bb = Buffer.from(b); return aa.length === bb.length && timingSafeEqual(aa, bb); }
function htmlEscape(value: string) { return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;").replaceAll("'", "&#039;"); }
function parseScope(scope?: string) { const requested = new Set((scope ?? SCOPE).split(/\s+/).filter(Boolean)); requested.add("mcp:tools"); requested.add("offline_access"); return [...requested].join(" "); }

export const oauthRouter = express.Router();
oauthRouter.use(express.urlencoded({ extended: false }));
oauthRouter.get("/.well-known/oauth-protected-resource", (_req, res) => res.json({ resource: MCP_RESOURCE, authorization_servers: [BASE_URL], scopes_supported: ["mcp:tools", "offline_access"], bearer_methods_supported: ["header"] }));
oauthRouter.get("/.well-known/oauth-authorization-server", (_req, res) => res.json({ issuer: BASE_URL, authorization_endpoint: `${BASE_URL}/authorize`, token_endpoint: `${BASE_URL}/token`, registration_endpoint: `${BASE_URL}/register`, response_types_supported: ["code"], grant_types_supported: ["authorization_code", "refresh_token"], code_challenge_methods_supported: ["S256"], token_endpoint_auth_methods_supported: ["none"], scopes_supported: ["mcp:tools", "offline_access"] }));

oauthRouter.post("/register", express.json({ limit: "64kb" }), async (req, res) => {
  await loadClients();
  const raw = Array.isArray(req.body?.redirect_uris) ? req.body.redirect_uris : [];
  const redirectUris: string[] = raw.filter((value: unknown): value is string => typeof value === "string");
  if (!redirectUris.length || redirectUris.some((redirectUri: string) => !validRedirectUri(redirectUri))) { res.status(400).json({ error: "invalid_redirect_uri" }); return; }
  const clientId = `codelocal_${randomBytes(24).toString("base64url")}`;
  const client: ClientRegistration = { clientId, redirectUris, clientName: typeof req.body?.client_name === "string" ? req.body.client_name : undefined };
  clients.set(clientId, client); await saveClients();
  res.status(201).json({ client_id: clientId, client_name: client.clientName ?? "MCP client", redirect_uris: redirectUris, grant_types: ["authorization_code", "refresh_token"], response_types: ["code"], token_endpoint_auth_method: "none" });
});

oauthRouter.get("/authorize", async (req, res) => {
  await loadClients();
  const clientId = String(req.query.client_id ?? ""); const redirectUri = String(req.query.redirect_uri ?? ""); const responseType = String(req.query.response_type ?? "");
  const codeChallenge = String(req.query.code_challenge ?? ""); const codeChallengeMethod = String(req.query.code_challenge_method ?? ""); const resource = String(req.query.resource ?? "");
  const scope = parseScope(typeof req.query.scope === "string" ? req.query.scope : undefined); const state = typeof req.query.state === "string" ? req.query.state : ""; const client = clients.get(clientId);
  if (!client || !client.redirectUris.includes(redirectUri) || responseType !== "code" || !codeChallenge || codeChallengeMethod !== "S256" || resource !== MCP_RESOURCE) { res.status(400).send("Invalid OAuth authorization request."); return; }
  const hidden = { client_id: clientId, redirect_uri: redirectUri, code_challenge: codeChallenge, resource, scope, state };
  const hiddenInputs = Object.entries(hidden).map(([k, v]) => `<input type="hidden" name="${k}" value="${htmlEscape(v)}">`).join("\n");
  res.type("html").send(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Authorize CodeLocal</title></head><body style="font-family:system-ui;background:#f6f6f6;display:grid;place-items:center;min-height:100vh"><div style="background:white;padding:28px;border-radius:16px;max-width:420px"><h2>Authorize CodeLocal</h2><p>Allow ChatGPT to access your paired CodeLocal coding workspaces.</p><form method="post" action="/authorize">${hiddenInputs}<input type="password" name="password" placeholder="CodeLocal passphrase" required style="width:100%;padding:12px;box-sizing:border-box"><button style="margin-top:12px;padding:12px;width:100%">Authorize</button></form></div></body></html>`);
});

oauthRouter.post("/authorize", async (req, res) => {
  await loadClients();
  const clientId = String(req.body.client_id ?? ""); const redirectUri = String(req.body.redirect_uri ?? ""); const codeChallenge = String(req.body.code_challenge ?? "");
  const resource = String(req.body.resource ?? ""); const scope = parseScope(String(req.body.scope ?? SCOPE)); const state = String(req.body.state ?? ""); const password = String(req.body.password ?? ""); const client = clients.get(clientId);
  if (!client || !client.redirectUris.includes(redirectUri) || !codeChallenge || resource !== MCP_RESOURCE) { res.status(400).send("Invalid OAuth authorization request."); return; }
  if (!equalSecret(password, MCP_USER_PASSWORD)) { res.status(401).send("Invalid CodeLocal passphrase."); return; }
  const code = randomBytes(32).toString("base64url"); authCodes.set(code, { clientId, redirectUri, codeChallenge, resource, scope, expiresAt: Date.now() + CODE_TTL_MS });
  const target = new URL(redirectUri); target.searchParams.set("code", code); if (state) target.searchParams.set("state", state); res.redirect(302, target.toString());
});

oauthRouter.post("/token", async (req, res) => {
  await loadClients(); res.setHeader("Cache-Control", "no-store"); res.setHeader("Pragma", "no-cache");
  const grantType = String(req.body.grant_type ?? ""); const clientId = String(req.body.client_id ?? ""); const resource = String(req.body.resource ?? "");
  try {
    if (grantType === "authorization_code") {
      const code = String(req.body.code ?? ""); const verifier = String(req.body.code_verifier ?? ""); const redirectUri = String(req.body.redirect_uri ?? ""); const record = authCodes.get(code); authCodes.delete(code);
      if (!record || record.expiresAt < Date.now() || record.clientId !== clientId || record.redirectUri !== redirectUri || record.resource !== resource) throw new Error("invalid_grant");
      const challenge = createHash("sha256").update(verifier).digest("base64url"); if (!verifier || challenge !== record.codeChallenge) throw new Error("invalid_grant");
      res.json(issueTokens(clientId, resource, record.scope)); return;
    }
    if (grantType === "refresh_token") {
      const payload = verifyToken(String(req.body.refresh_token ?? ""), "refresh"); if (payload.client_id !== clientId || payload.resource !== resource) throw new Error("invalid_grant");
      res.json(issueTokens(clientId, resource, payload.scope)); return;
    }
    res.status(400).json({ error: "unsupported_grant_type" });
  } catch { res.status(400).json({ error: "invalid_grant" }); }
});

function unauthorized(res: Response) { res.setHeader("WWW-Authenticate", `Bearer realm="codelocal", resource_metadata="${BASE_URL}/.well-known/oauth-protected-resource"`); res.status(401).json({ error: "unauthorized" }); }
export function requireMcpAuth(req: Request, res: Response, next: NextFunction) {
  const header = req.headers.authorization; if (!header?.startsWith("Bearer ")) { unauthorized(res); return; }
  try { const payload = verifyToken(header.slice(7), "access"); if (!payload.scope.split(/\s+/).includes("mcp:tools")) { res.status(403).json({ error: "insufficient_scope" }); return; } res.locals.oauth = payload; next(); }
  catch { unauthorized(res); }
}
