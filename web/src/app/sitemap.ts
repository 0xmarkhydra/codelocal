import type { MetadataRoute } from "next";
import { taxonomySlug } from "@/lib/blog";
import { getBlogPostsForRender, getBlogSeriesForRender } from "@/lib/blog-server";

const SITE_URL = (process.env.NEXT_PUBLIC_SITE_URL || "https://codelocal.cloud").replace(/\/$/, "");

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const [posts, series] = await Promise.all([getBlogPostsForRender(), getBlogSeriesForRender()]);
  const categories = [...new Set(posts.map((post) => post.category))].sort();
  const tags = [...new Set(posts.flatMap((post) => post.tags))].sort();
  const authors = [...new Set(posts.map((post) => post.author.slug))];

  const staticRoutes: MetadataRoute.Sitemap = [
    { url: SITE_URL, priority: 1 },
    { url: `${SITE_URL}/blogs`, priority: 0.9 },
    { url: `${SITE_URL}/blogs/series`, priority: 0.75 },
    { url: `${SITE_URL}/security`, priority: 0.7 },
    { url: `${SITE_URL}/support`, priority: 0.6 },
  ];

  const postRoutes: MetadataRoute.Sitemap = posts.map((post) => ({
    url: `${SITE_URL}/blogs/${post.slug}`,
    lastModified: new Date(`${post.updatedAt ?? post.publishedAt}T00:00:00Z`),
    priority: post.featured ? 0.85 : 0.8,
  }));

  const seriesRoutes: MetadataRoute.Sitemap = series.map((item) => ({
    url: `${SITE_URL}/blogs/series/${item.slug}`,
    priority: 0.75,
  }));

  const categoryRoutes: MetadataRoute.Sitemap = categories.map((category) => ({
    url: `${SITE_URL}/blogs/category/${taxonomySlug(category)}`,
    priority: 0.55,
  }));

  const tagRoutes: MetadataRoute.Sitemap = tags.map((tag) => ({
    url: `${SITE_URL}/blogs/tag/${taxonomySlug(tag)}`,
    priority: 0.5,
  }));

  const authorRoutes: MetadataRoute.Sitemap = authors.map((author) => ({
    url: `${SITE_URL}/users/${encodeURIComponent(author)}`,
    priority: 0.45,
  }));

  return [...staticRoutes, ...postRoutes, ...seriesRoutes, ...categoryRoutes, ...tagRoutes, ...authorRoutes];
}
