import "server-only";

import { createHash } from "node:crypto";

const shortCodeLength = 8;

export function blogShortCode(slug: string) {
  return createHash("sha256")
    .update(slug, "utf8")
    .digest("base64url")
    .slice(0, shortCodeLength);
}
