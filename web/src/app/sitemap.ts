import type { MetadataRoute } from "next";
import {
  blogPosts,
  blogSeries,
  getCategories,
  getTags,
  taxonomySlug,
} from "@/lib/blog";

const SITE_URL = (process.env.NEXT_PUBLIC_SITE_URL || "https://codelocal.cloud").replace(/\/$/, "");

export default function sitemap(): MetadataRoute.Sitemap {
  const staticRoutes: MetadataRoute.Sitemap = [
    { url: SITE_URL, priority: 1 },
    { url: `${SITE_URL}/blog`, priority: 0.9 },
    { url: `${SITE_URL}/blog/series`, priority: 0.75 },
    { url: `${SITE_URL}/security`, priority: 0.7 },
    { url: `${SITE_URL}/support`, priority: 0.6 },
  ];

  const postRoutes: MetadataRoute.Sitemap = blogPosts.map((post) => ({
    url: `${SITE_URL}/blog/${post.slug}`,
    lastModified: new Date(`${post.updatedAt ?? post.publishedAt}T00:00:00Z`),
    priority: post.featured ? 0.85 : 0.8,
  }));

  const seriesRoutes: MetadataRoute.Sitemap = blogSeries.map((series) => ({
    url: `${SITE_URL}/blog/series/${series.slug}`,
    priority: 0.75,
  }));

  const categoryRoutes: MetadataRoute.Sitemap = getCategories().map((category) => ({
    url: `${SITE_URL}/blog/category/${taxonomySlug(category)}`,
    priority: 0.55,
  }));

  const tagRoutes: MetadataRoute.Sitemap = getTags().map((tag) => ({
    url: `${SITE_URL}/blog/tag/${taxonomySlug(tag)}`,
    priority: 0.5,
  }));

  return [...staticRoutes, ...postRoutes, ...seriesRoutes, ...categoryRoutes, ...tagRoutes];
}
