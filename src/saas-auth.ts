import { randomBytes, scrypt as scryptCallback, timingSafeEqual } from "node:crypto";
import { promisify } from "node:util";
import express, { type NextFunction, type Request, type Response } from "express";
import { cloudStore, type CloudUser } from "./cloud-store.js";
import { authPage, escapeHtml } from "./web-ui.js";

const scrypt = promisify(scryptCallback);
const SESSION_COOKIE = "codelocal_session";
const CSRF_COOKIE = "codelocal_csrf";
const SESSION_TTL_SECONDS = 30 * 24 * 60 * 60;
const DUMMY_LOGIN_SALT = "codelocal-login-timing-padding-v1";

export type WebIdentity = {
  user: CloudUser;
  sessionId: string;
  csrf: string;
};

function parseCookies(req: Request) {
  const header = req.headers.cookie ?? "";
  const values = new Map<string, string>();
  for (const part of header.split(";")) {
    const index = part.indexOf("=");
    if (index <= 0) continue;
    const key = part.slice(0, index).trim();
    const value = part.slice(index + 1).trim();
    try { values.set(key, decodeURIComponent(value)); } catch { values.set(key, value); }
  }
  return values;
}

function secureCookies() {
  return (process.env.PUBLIC_BASE_URL ?? "").startsWith("https://") || process.env.NODE_ENV === "production";
}

function cookie(name: string, value: string, options: { httpOnly?: boolean; maxAge?: number } = {}) {
  const fields = [`${name}=${encodeURIComponent(value)}`, "Path=/", "SameSite=Lax"];
  if (options.httpOnly !== false) fields.push("HttpOnly");
  if (secureCookies()) fields.push("Secure");
  if (options.maxAge != null) fields.push(`Max-Age=${Math.max(0, Math.floor(options.maxAge))}`);
  return fields.join("; ");
}

function clearCookie(name: string) {
  return cookie(name, "", { maxAge: 0 });
}

function safeNext(value: unknown) {
  const next = typeof value === "string" ? value : "/dashboard";
  if (!next.startsWith("/") || next.startsWith("//") || next.includes("\\")) return "/dashboard";
  return next;
}

function validEmail(value: string) {
  return value.length <= 254 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

async function derivePassword(password: string, salt: string) {
  const result = await scrypt(password, salt, 64) as Buffer;
  return result.toString("base64url");
}

export async function hashPassword(password: string) {
  if (password.length < 10) throw new Error("Password must be at least 10 characters.");
  if (password.length > 256) throw new Error("Password is too long.");
  const salt = randomBytes(24).toString("base64url");
  return { salt, hash: await derivePassword(password, salt) };
}

export async function verifyPassword(password: string, salt: string, expectedHash: string) {
  if (password.length > 256) return false;
  const actual = Buffer.from(await derivePassword(password, salt));
  const expected = Buffer.from(expectedHash);
  return actual.length === expected.length && timingSafeEqual(actual, expected);
}

export function ensureCsrf(req: Request, res: Response) {
  const existing = parseCookies(req).get(CSRF_COOKIE);
  if (existing && /^[A-Za-z0-9_-]{24,128}$/.test(existing)) return existing;
  const value = randomBytes(32).toString("base64url");
  res.append("Set-Cookie", cookie(CSRF_COOKIE, value, { maxAge: SESSION_TTL_SECONDS }));
  return value;
}

export function verifyCsrf(req: Request) {
  const cookieValue = parseCookies(req).get(CSRF_COOKIE) ?? "";
  const bodyValue = typeof req.body?.csrf === "string" ? req.body.csrf : "";
  const aa = Buffer.from(cookieValue);
  const bb = Buffer.from(bodyValue);
  return aa.length > 0 && aa.length === bb.length && timingSafeEqual(aa, bb);
}

export async function getWebIdentity(req: Request): Promise<WebIdentity | null> {
  const cached = (req as any).__codelocalIdentity;
  if (cached !== undefined) return cached;
  const sessionId = parseCookies(req).get(SESSION_COOKIE) ?? "";
  const session = await cloudStore.readSession(sessionId);
  if (!session) { (req as any).__codelocalIdentity = null; return null; }
  const user = await cloudStore.userById(session.userId);
  if (!user) { (req as any).__codelocalIdentity = null; return null; }
  const identity = { user, sessionId, csrf: session.csrf };
  (req as any).__codelocalIdentity = identity;
  return identity;
}

export async function requireWebUser(req: Request, res: Response, next: NextFunction) {
  const identity = await getWebIdentity(req);
  if (!identity) {
    const nextUrl = safeNext(req.originalUrl || "/dashboard");
    res.redirect(302, `/login?next=${encodeURIComponent(nextUrl)}`);
    return;
  }
  res.locals.webIdentity = identity;
  next();
}

function authForm(mode: "login" | "signup", csrf: string, next: string, error?: string) {
  const signup = mode === "signup";
  return authPage({
    title: signup ? "Create your CodeLocal account" : "Welcome back",
    subtitle: signup ? "One account connects ChatGPT to your development machines." : "Sign in to manage your devices, workspaces and MCP extensions.",
    body: `${error ? `<div class="alert">${escapeHtml(error)}</div><div style="height:14px"></div>` : ""}<form class="form" method="post" action="/${mode}">
      <input type="hidden" name="csrf" value="${escapeHtml(csrf)}">
      <input type="hidden" name="next" value="${escapeHtml(next)}">
      <div class="field"><label>Email</label><input class="input" type="email" name="email" autocomplete="email" maxlength="254" required autofocus></div>
      <div class="field"><label>Password</label><input class="input" type="password" name="password" autocomplete="${signup ? "new-password" : "current-password"}" minlength="10" maxlength="256" required></div>
      ${signup ? `<div class="hint">Use at least 10 characters. Passwords are scrypt-hashed; CodeLocal never stores the original password.</div>` : ""}
      <button class="btn primary" type="submit">${signup ? "Create account" : "Sign in"}</button>
    </form><div class="auth-switch">${signup ? `Already have an account? <a href="/login?next=${encodeURIComponent(next)}">Sign in</a>` : `New to CodeLocal? <a href="/signup?next=${encodeURIComponent(next)}">Create an account</a>`}</div>`,
  });
}

export const webAuthRouter = express.Router();
webAuthRouter.use(express.urlencoded({ extended: false, limit: "64kb" }));

webAuthRouter.get("/login", async (req, res) => {
  if (await getWebIdentity(req)) { res.redirect(302, safeNext(req.query.next)); return; }
  const csrf = ensureCsrf(req, res);
  res.type("html").send(authForm("login", csrf, safeNext(req.query.next)));
});

webAuthRouter.post("/login", async (req, res) => {
  const csrf = ensureCsrf(req, res);
  const next = safeNext(req.body?.next);
  if (!verifyCsrf(req)) { res.status(403).type("html").send(authForm("login", csrf, next, "Security token expired. Please try again.")); return; }
  const email = String(req.body?.email ?? "").trim().toLowerCase();
  const password = String(req.body?.password ?? "");
  const user = validEmail(email) ? await cloudStore.userByEmail(email) : null;
  let valid = false;
  if (user) valid = await verifyPassword(password, user.passwordSalt, user.passwordHash).catch(() => false);
  else if (password.length <= 256) await derivePassword(password, DUMMY_LOGIN_SALT).catch(() => undefined);
  if (!user || !valid) {
    await cloudStore.audit(user?.id, "auth.login_failed", { email });
    res.status(401).type("html").send(authForm("login", csrf, next, "Email or password is incorrect."));
    return;
  }
  const sessionId = await cloudStore.createSession(user.id, csrf, SESSION_TTL_SECONDS);
  res.append("Set-Cookie", cookie(SESSION_COOKIE, sessionId, { maxAge: SESSION_TTL_SECONDS }));
  await cloudStore.audit(user.id, "auth.login", { method: "password" });
  res.redirect(303, next);
});

webAuthRouter.get("/register", (req, res) => {
  const next = safeNext(req.query.next);
  res.redirect(302, `/signup?next=${encodeURIComponent(next)}`);
});

webAuthRouter.get("/signup", async (req, res) => {
  if (await getWebIdentity(req)) { res.redirect(302, safeNext(req.query.next)); return; }
  const csrf = ensureCsrf(req, res);
  res.type("html").send(authForm("signup", csrf, safeNext(req.query.next)));
});

webAuthRouter.post("/signup", async (req, res) => {
  const csrf = ensureCsrf(req, res);
  const next = safeNext(req.body?.next);
  if (!verifyCsrf(req)) { res.status(403).type("html").send(authForm("signup", csrf, next, "Security token expired. Please try again.")); return; }
  const email = String(req.body?.email ?? "").trim().toLowerCase();
  const password = String(req.body?.password ?? "");
  if (!validEmail(email)) { res.status(400).type("html").send(authForm("signup", csrf, next, "Enter a valid email address.")); return; }
  try {
    const passwordData = await hashPassword(password);
    const user = await cloudStore.createUser(email, passwordData.hash, passwordData.salt);
    const sessionId = await cloudStore.createSession(user.id, csrf, SESSION_TTL_SECONDS);
    res.append("Set-Cookie", cookie(SESSION_COOKIE, sessionId, { maxAge: SESSION_TTL_SECONDS }));
    await cloudStore.audit(user.id, "auth.register", {});
    res.redirect(303, next);
  } catch (error) {
    const message = error instanceof Error && error.message === "EMAIL_ALREADY_REGISTERED" ? "An account with this email already exists." : (error instanceof Error ? error.message : "Unable to create account.");
    res.status(400).type("html").send(authForm("signup", csrf, next, message));
  }
});

webAuthRouter.post("/logout", async (req, res) => {
  const identity = await getWebIdentity(req);
  if (!verifyCsrf(req)) { res.status(403).send("Invalid security token."); return; }
  if (identity) {
    await cloudStore.deleteSession(identity.sessionId);
    await cloudStore.audit(identity.user.id, "auth.logout", {});
  }
  res.append("Set-Cookie", clearCookie(SESSION_COOKIE));
  res.redirect(303, "/login");
});
