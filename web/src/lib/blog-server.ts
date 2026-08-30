import "server-only";

import {
  blogPosts as seedPosts,
  blogSeries as seedSeries,
  getPostBySlug,
  getSeriesBySlug,
  getSeriesPosts,
  taxonomySlug,
  type BlogAuthor,
  type BlogBlock,
  type BlogPost,
  type BlogSeries,
} from "@/lib/blog";
import {
  isBlogPostResource,
  isPublicBlogPostsResource,
  isPublicBlogSeriesCollectionResource,
  isPublicBlogSeriesResource,
  type BlogPostSummary as DurablePostSummary,
  type BlogSeries as DurableSeries,
} from "@/lib/contracts/blog";

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
    const response = await fetch(`${origin}${path}`, {
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return undefined;
    return response.json();
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

function wordCount(value: string) {
  return value.split(/\s+/).filter(Boolean).length;
}

function readingMinutes(blocks: BlogBlock[], fallbackText = "") {
  const words = blocks.reduce((total, block) => {
    if (block.type === "image") return total;
    if (block.type === "list") return total + wordCount(block.items.join(" "));
    if (block.type === "code") return total + wordCount(block.code);
    if (block.type === "callout") return total + wordCount(`${block.title} ${block.text}`);
    return total + wordCount(block.text);
  }, wordCount(fallbackText));
  return Math.max(1, Math.ceil(words / 220));
}

function durableAuthor(userID: string, official: boolean): BlogAuthor {
  return {
    slug: userID,
    name: official ? "CodeLocal Team" : "CodeLocal Creator",
    role: official ? "Engineering & Product" : "CodeLocal Community",
  };
}

function seriesByID(items: DurableSeries[]) {
  return new Map(items.map((series) => [series.id, series]));
}

function durableSeriesToRender(series: DurableSeries): BlogSeries {
  const official = Boolean(series.official);
  return {
    slug: series.slug,
    title: series.title,
    description: series.description,
    status: series.status === "complete" ? "complete" : "active",
    category: official ? "CodeLocal" : "Community",
    coverAssetId: series.coverAssetId,
    author: durableAuthor(series.authorUserId, official),
  };
}

function durableSummaryToRender(post: DurablePostSummary, seriesMap: Map<string, DurableSeries>): BlogPost {
  const linkedSeries = post.seriesId ? seriesMap.get(post.seriesId) : undefined;
  const fallback = `${post.title} ${post.excerpt}`;
  return {
    slug: post.slug,
    title: post.title,
    excerpt: post.excerpt,
    publishedAt: dateOnly(post.publishedAt ?? post.updatedAt),
    updatedAt: dateOnly(post.updatedAt),
    readingMinutes: readingMinutes([], fallback),
    category: post.category || "Community",
    tags: post.tags,
    author: durableAuthor(post.authorUserId, post.official),
    official: post.official,
    featured: post.featured,
    showOnLanding: post.showOnLanding,
    coverAssetId: post.coverAssetId,
    series: linkedSeries && post.seriesPart ? { slug: linkedSeries.slug, part: post.seriesPart } : undefined,
    blocks: [],
  };
}

function mergeBySlug<T extends { slug: string }>(seed: T[], durable: T[]) {
  const merged = new Map(seed.map((item) => [item.slug, item]));
  for (const item of durable) merged.set(item.slug, item);
  return [...merged.values()];
}

async function publicSeriesCollection(): Promise<DurableSeries[]> {
  const body = await backendJSON("/api/v1/blog/public/series");
  return isPublicBlogSeriesCollectionResource(body) ? body.series : [];
}

async function durablePost(slug: string): Promise<BlogPost | undefined> {
  const [body, series] = await Promise.all([
    backendJSON(`/api/v1/blog/public/${encodeURIComponent(slug)}`),
    publicSeriesCollection(),
  ]);
  if (!isBlogPostResource(body)) return undefined;
  const blocks = body.post.content.map(toBlogBlock).filter((block): block is BlogBlock => Boolean(block));
  const linkedSeries = body.post.seriesId ? seriesByID(series).get(body.post.seriesId) : undefined;
  const official = Boolean(body.official);
  return {
    slug: body.post.slug,
    title: body.post.title,
    excerpt: body.post.excerpt,
    publishedAt: dateOnly(body.post.publishedAt ?? body.post.updatedAt),
    updatedAt: dateOnly(body.post.updatedAt),
    readingMinutes: readingMinutes(blocks, `${body.post.title} ${body.post.excerpt}`),
    category: body.post.category || "Community",
    tags: body.post.tags,
    author: durableAuthor(body.post.authorUserId, official),
    official,
    featured: body.post.featured,
    showOnLanding: body.post.showOnLanding,
    coverAssetId: body.post.coverAssetId,
    series: linkedSeries && body.post.seriesPart ? { slug: linkedSeries.slug, part: body.post.seriesPart } : undefined,
    blocks,
  };
}

export async function getBlogPostForRender(slug: string) {
  return (await durablePost(slug)) ?? getPostBySlug(slug);
}

export async function getBlogPostsForRender() {
  const [postsBody, durableSeries] = await Promise.all([
    backendJSON("/api/v1/blog/public"),
    publicSeriesCollection(),
  ]);
  if (!isPublicBlogPostsResource(postsBody)) return seedPosts;
  const lookup = seriesByID(durableSeries);
  const durablePosts = postsBody.posts.map((post) => durableSummaryToRender(post, lookup));
  return mergeBySlug(seedPosts, durablePosts).sort((a, b) => b.publishedAt.localeCompare(a.publishedAt));
}

export async function getBlogSeriesForRender() {
  const durable = (await publicSeriesCollection()).map(durableSeriesToRender);
  return mergeBySlug(seedSeries, durable).sort((a, b) => a.title.localeCompare(b.title));
}

export async function getBlogSeriesPageForRender(slug: string): Promise<{ series: BlogSeries; posts: BlogPost[]; redirected: boolean } | undefined> {
  const body = await backendJSON(`/api/v1/blog/public/series/${encodeURIComponent(slug)}`);
  if (isPublicBlogSeriesResource(body)) {
    const series = durableSeriesToRender(body.series);
    const lookup = new Map([[body.series.id, body.series]]);
    return {
      series,
      posts: body.posts.map((post) => durableSummaryToRender(post, lookup)).sort((a, b) => (a.series?.part ?? 0) - (b.series?.part ?? 0)),
      redirected: body.redirected,
    };
  }
  const local = getSeriesBySlug(slug);
  if (!local) return undefined;
  return { series: local, posts: getSeriesPosts(local.slug), redirected: false };
}

export async function searchBlogPostsForRender(query: string) {
  const normalized = query.trim().toLowerCase();
  const posts = await getBlogPostsForRender();
  if (!normalized) return posts;
  return posts.filter((post) => [post.title, post.excerpt, post.category, ...post.tags].join(" ").toLowerCase().includes(normalized));
}

export async function getPostsByCategorySlugForRender(slug: string) {
  return (await getBlogPostsForRender()).filter((post) => taxonomySlug(post.category) === slug);
}

export async function getPostsByTagSlugForRender(slug: string) {
  return (await getBlogPostsForRender()).filter((post) => post.tags.some((tag) => taxonomySlug(tag) === slug));
}

export async function getBlogCategoriesForRender() {
  return [...new Set((await getBlogPostsForRender()).map((post) => post.category))].sort();
}

export async function getBlogTagsForRender() {
  return [...new Set((await getBlogPostsForRender()).flatMap((post) => post.tags))].sort();
}

export async function getRelatedPostsForRender(post: BlogPost, limit = 3) {
  return (await getBlogPostsForRender())
    .filter((candidate) => candidate.slug !== post.slug)
    .map((candidate) => {
      const sharedTags = candidate.tags.filter((tag) => post.tags.includes(tag)).length;
      const sameCategory = candidate.category === post.category ? 2 : 0;
      const sameSeries = candidate.series?.slug && candidate.series.slug === post.series?.slug ? 3 : 0;
      return { candidate, score: sharedTags + sameCategory + sameSeries };
    })
    .sort((a, b) => b.score - a.score || b.candidate.publishedAt.localeCompare(a.candidate.publishedAt))
    .slice(0, limit)
    .map(({ candidate }) => candidate);
}

export async function getBlogPostsByAuthorForRender(authorSlug: string) {
  return (await getBlogPostsForRender()).filter((post) => post.author.slug === authorSlug);
}
