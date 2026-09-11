import test from "node:test";
import assert from "node:assert/strict";
import { cloudStore } from "../cloud-store.js";
import { rateLimit } from "../rate-limit.js";

test("rate limit middleware allows requests under the limit and returns 429 when exceeded", async () => {
  const original = cloudStore.rateLimit.bind(cloudStore);
  const calls: Array<{ scope: string; subject: string; limit: number; windowSeconds: number }> = [];
  try {
    (cloudStore as any).rateLimit = async (scope: string, subject: string, limit: number, windowSeconds: number) => {
      calls.push({ scope, subject, limit, windowSeconds });
      return calls.length === 1
        ? { allowed: true, count: 1, limit, retryAfterSeconds: 0 }
        : { allowed: false, count: limit + 1, limit, retryAfterSeconds: 17 };
    };

    const middleware = rateLimit({ scope: "test", limit: 2, windowSeconds: 60, subject: () => "subject" });
    const req = { headers: {}, socket: { remoteAddress: "127.0.0.1" } } as any;

    const headers = new Map<string, string>();
    let statusCode = 200;
    let body: any = null;
    const res = {
      setHeader(name: string, value: string) { headers.set(name, value); },
      status(code: number) { statusCode = code; return this; },
      json(value: unknown) { body = value; return this; },
    } as any;

    let nextCalls = 0;
    await middleware(req, res, () => { nextCalls++; });
    assert.equal(nextCalls, 1);
    assert.equal(headers.get("RateLimit-Limit"), "2");
    assert.equal(headers.get("RateLimit-Remaining"), "1");

    await middleware(req, res, () => { nextCalls++; });
    assert.equal(nextCalls, 1);
    assert.equal(statusCode, 429);
    assert.equal(headers.get("Retry-After"), "17");
    assert.deepEqual(body, { error: "rate_limited", retryAfterSeconds: 17 });
    assert.equal(calls[0]?.subject, "subject");
  } finally {
    (cloudStore as any).rateLimit = original;
  }
});
