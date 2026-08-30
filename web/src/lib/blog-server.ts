import "server-only";

import { getPostBySlug, type BlogBlock, type BlogPost } from "@/lib/blog";
import { isBlogPostResource } from "@/lib/contracts/blog";

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

function positiveDimension(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? Math.round(value) : fallback;
}

function toBlogBlock(value: unknown): BlogBlock | undefined {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return undefined;
  const block = value as Record<string, unknown>;
  if (block.type === "paragraph" && typeof block.text === "string") return { type: "paragraph", text: block.text };
  if (block.type === "heading" && typeof block.text === "string") return { type: "heading", text: block.text };
  if (block.type === "list" && Array.isArray(block.items)) {
    return { type: "list", items: block.items.filter((item): item is string => typeof item === "string") };
  }
  if (block.type === "code" && typeof block.code === "string") {
    return { type: "code", code: block.code, language: typeof block.language === "string" ? block.language : undefined };
  }
  if (block.type === "callout" && typeof block.title === "string" && typeof block.text === "string") {
    return { type: "callout", title: block.title, text: block.text };
  }
  if (block.type === "image" && typeof block.assetId === "string" && block.assetId.startsWith("media_")) {
    return {
      type: "image",
      assetId: block.assetId,
      width: positiveDimension(block.width, 1600),
      height: positiveDimension(block.height, 900),
      alt: typeof block.alt === "string" ? block.alt : "",
      caption: typeof block.caption === "string" ? block.caption : undefined,
      variant: block.variant === "medium" ? "medium" : "large",
    };
  }
  return undefined;
}

function dateOnly(value: number) {
  return new Date(value || Date.now()).toISOString().slice(0, 10);
}

function readingMinutes(blocks: BlogBlock[]) {
  const words = blocks.reduce((total, block) => {
    if (block.type === "image") return total;
    if (block.type === "list") return total + block.items.join(" ").split(/\s+/).filter(Boolean).length;
    if (block.type === "code") return total + block.code.split(/\s+/).filter(Boolean).length;
    if (block.type === "callout") return total + `${block.title} ${block.text}`.split(/\s+/).filter(Boolean).length;
    return total + block.text.split(/\s+/).filter(Boolean).length;
  }, 0);
  return Math.max(1, Math.ceil(words / 220));
}

async function durablePost(slug: string): Promise<BlogPost | undefined> {
  const origin = backendOrigin();
  if (!origin) return undefined;
  try {
    const response = await fetch(`${origin}/api/v1/blog/public/${encodeURIComponent(slug)}`, {
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return undefined;
    const body: unknown = await response.json();
    if (!isBlogPostResource(body)) return undefined;
    const blocks = body.post.content.map(toBlogBlock).filter((block): block is BlogBlock => Boolean(block));
    const authorLabel = body.official ? "CodeLocal Team" : (body.post.authorEmail ? body.post.authorEmail.split("@")[0] : "CodeLocal Creator");
    return {
      slug: body.post.slug,
      title: body.post.title,
      excerpt: body.post.excerpt,
      publishedAt: dateOnly(body.post.publishedAt ?? body.post.updatedAt),
      updatedAt: dateOnly(body.post.updatedAt),
      readingMinutes: readingMinutes(blocks),
      category: body.post.category || "Community",
      tags: body.post.tags,
      author: {
        slug: body.post.authorUserId,
        name: authorLabel,
        role: body.official ? "Engineering & Product" : "CodeLocal Community",
      },
      featured: body.post.featured,
      coverAssetId: body.post.coverAssetId,
      blocks,
    };
  } catch {
    return undefined;
  }
}

export async function getBlogPostForRender(slug: string) {
  const local = getPostBySlug(slug);
  if (local) return local;
  return durablePost(slug);
}
