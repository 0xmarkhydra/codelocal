import "server-only";

import { createHash, createHmac, timingSafeEqual } from "node:crypto";
import { cookies } from "next/headers";

const SESSION_COOKIE = "codelocal_pool_session";
const SESSION_TTL_SECONDS = 8 * 60 * 60;

function requiredEnv(name: string): string {
  const value = (process.env[name] || "").trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}

function sessionSecret(): string {
  const value = requiredEnv("POOL_WEB_SESSION_SECRET");
  if (Buffer.byteLength(value, "utf8") < 32) throw new Error("POOL_WEB_SESSION_SECRET must be at least 32 bytes");
  return value;
}

function digest(value: string): Buffer { return createHash("sha256").update(value).digest(); }
function equalSecret(left: string, right: string): boolean { return timingSafeEqual(digest(left), digest(right)); }
function signature(payload: string): string { return createHmac("sha256", sessionSecret()).update(payload).digest("hex"); }
function tokenFor(expirySeconds: number): string {
  const payload = `v1.${expirySeconds}`;
  return `${payload}.${signature(payload)}`;
}
function validToken(token: string | undefined): boolean {
  if (!token) return false;
  const parts = token.split(".");
  if (parts.length !== 3 || parts[0] !== "v1") return false;
  const expiry = Number(parts[1]);
  if (!Number.isSafeInteger(expiry) || expiry <= Math.floor(Date.now() / 1000)) return false;
  const payload = `v1.${expiry}`;
  return equalSecret(parts[2], signature(payload));
}

export function verifyPoolPassword(candidate: string): boolean { return equalSecret(candidate, requiredEnv("POOL_WEB_PASSWORD")); }
export async function hasPoolSession(): Promise<boolean> {
  const store = await cookies();
  return validToken(store.get(SESSION_COOKIE)?.value);
}
export async function createPoolSession(): Promise<void> {
  const expiry = Math.floor(Date.now() / 1000) + SESSION_TTL_SECONDS;
  const store = await cookies();
  store.set(SESSION_COOKIE, tokenFor(expiry), {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "strict",
    path: "/",
    maxAge: SESSION_TTL_SECONDS,
  });
}
export async function deletePoolSession(): Promise<void> {
  const store = await cookies();
  store.set(SESSION_COOKIE, "", {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "strict",
    path: "/",
    maxAge: 0,
  });
}
