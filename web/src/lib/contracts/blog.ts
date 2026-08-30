export type BlogPostSummary = {
  id: string;
  slug: string;
  authorUserId: string;
  authorEmail?: string;
  title: string;
  excerpt: string;
  coverAssetId?: string;
  category?: string;
  tags: string[];
  seriesId?: string;
  seriesPart?: number;
  status: "draft" | "scheduled" | "published" | "archived";
  visibility: "public" | "unlisted" | "private";
  moderationStatus: "clean" | "pending" | "hidden";
  featured: boolean;
  showOnLanding: boolean;
  official: boolean;
  publishedAt?: number;
  scheduledAt?: number;
  createdAt: number;
  updatedAt: number;
};

export type BlogPost = Omit<BlogPostSummary, "official"> & {
  content: unknown[];
};

export type BlogPostsResource = {
  posts: BlogPostSummary[];
  isAdmin: boolean;
};

export type BlogPostResource = {
  post: BlogPost;
  official?: boolean;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function isNonNegativeNumber(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function isBlogPostSummary(value: unknown): value is BlogPostSummary {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" && value.id.length > 0 &&
    typeof value.slug === "string" && value.slug.length > 0 &&
    typeof value.authorUserId === "string" && value.authorUserId.length > 0 &&
    typeof value.title === "string" &&
    typeof value.excerpt === "string" &&
    isStringArray(value.tags) &&
    ["draft", "scheduled", "published", "archived"].includes(String(value.status)) &&
    ["public", "unlisted", "private"].includes(String(value.visibility)) &&
    ["clean", "pending", "hidden"].includes(String(value.moderationStatus)) &&
    typeof value.featured === "boolean" &&
    typeof value.showOnLanding === "boolean" &&
    typeof value.official === "boolean" &&
    isNonNegativeNumber(value.createdAt) && isNonNegativeNumber(value.updatedAt)
  );
}

export function isBlogPostsResource(value: unknown): value is BlogPostsResource {
  if (!isRecord(value) || !Array.isArray(value.posts) || typeof value.isAdmin !== "boolean") return false;
  return value.posts.every(isBlogPostSummary);
}

export function isBlogPostResource(value: unknown): value is BlogPostResource {
  if (!isRecord(value) || !isRecord(value.post)) return false;
  const post = value.post;
  const withOfficial = { ...post, official: typeof value.official === "boolean" ? value.official : false };
  return isBlogPostSummary(withOfficial) && Array.isArray(post.content);
}
