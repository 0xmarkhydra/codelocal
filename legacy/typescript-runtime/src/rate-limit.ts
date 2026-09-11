import type { NextFunction, Request, Response } from "express";
import { cloudStore } from "./cloud-store.js";

export type RateLimitOptions = {
  scope: string;
  limit: number;
  windowSeconds: number;
  subject?: (req: Request) => string;
};

function trustedProxyHeaders() {
  return process.env.CODELOCAL_TRUST_PROXY === "1"
    || Boolean(process.env.RAILWAY_ENVIRONMENT)
    || Boolean(process.env.RAILWAY_ENVIRONMENT_ID)
    || Boolean(process.env.RAILWAY_PROJECT_ID);
}

export function requestClientIp(req: Request) {
  if (trustedProxyHeaders()) {
    const forwarded = req.headers["x-forwarded-for"];
    const first = Array.isArray(forwarded) ? forwarded[0] : forwarded?.split(",")[0];
    if (first?.trim()) return first.trim().slice(0, 128);
  }
  return String(req.socket.remoteAddress ?? req.ip ?? "unknown").slice(0, 128);
}

export function rateLimit(options: RateLimitOptions) {
  return async (req: Request, res: Response, next: NextFunction) => {
    try {
      const subject = options.subject?.(req) || requestClientIp(req);
      const result = await cloudStore.rateLimit(options.scope, subject, options.limit, options.windowSeconds);
      res.setHeader("RateLimit-Limit", String(result.limit));
      res.setHeader("RateLimit-Remaining", String(Math.max(0, result.limit - result.count)));
      if (result.allowed) { next(); return; }
      res.setHeader("Retry-After", String(result.retryAfterSeconds));
      res.status(429).json({ error: "rate_limited", retryAfterSeconds: result.retryAfterSeconds });
    } catch (error) {
      console.error(JSON.stringify({ ts: new Date().toISOString(), level: "error", event: "rate_limit.error", scope: options.scope, error: String(error) }));
      res.status(503).json({ error: "rate_limit_unavailable" });
    }
  };
}
