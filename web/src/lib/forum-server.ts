import "server-only";

import { isPublicForumCollection, isPublicForumTopicResource, type PublicForumTopic } from "@/lib/contracts/forum";

function backendOrigin() {
  const raw = (process.env.CODELOCAL_BACKEND_URL || "http://127.0.0.1:3333").trim();
  try {
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:") return undefined;
    url.pathname = "";
    url.search = "";
    url.hash = "";
    return url.toString().replace(/\/$/, "");
  } catch {
    return undefined;
  }
}

async function backendJSON(path: string): Promise<unknown | undefined> {
  const origin = backendOrigin();
  if (!origin) return undefined;
  try {
    const response = await fetch(`${origin}${path}`, { cache: "no-store", headers: { Accept: "application/json" } });
    if (!response.ok) return undefined;
    return response.json();
  } catch {
    return undefined;
  }
}

export async function getPublicForumTopics(input: { kind?: string; status?: string; q?: string; limit?: number } = {}): Promise<PublicForumTopic[]> {
  const params = new URLSearchParams();
  if (input.kind) params.set("kind", input.kind);
  if (input.status) params.set("status", input.status);
  if (input.q) params.set("q", input.q);
  params.set("limit", String(input.limit ?? 100));
  const body = await backendJSON(`/api/v1/public/forums/topics?${params.toString()}`);
  return isPublicForumCollection(body) ? body.topics : [];
}

export async function getPublicForumTopic(topicID: string) {
  const body = await backendJSON(`/api/v1/public/forums/topics/${encodeURIComponent(topicID)}`);
  return isPublicForumTopicResource(body) ? body : undefined;
}

export function publicForumImageURL(assetID: string, variant: "thumb" | "medium" | "large" = "large") {
  return `/api/v1/public/media/${encodeURIComponent(assetID)}/${variant}`;
}

export function forumExcerpt(value: string, limit = 180) {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= limit) return normalized;
  return `${normalized.slice(0, limit - 1).trimEnd()}…`;
}
