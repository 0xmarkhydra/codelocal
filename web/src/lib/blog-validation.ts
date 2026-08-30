import { blogPosts, blogSeries } from "./blog";

function assertUnique(values: string[], label: string) {
  const seen = new Set<string>();
  for (const value of values) {
    if (seen.has(value)) throw new Error(`Duplicate blog ${label}: ${value}`);
    seen.add(value);
  }
}

export function validateBlogRegistry() {
  assertUnique(blogPosts.map((post) => post.slug), "post slug");
  assertUnique(blogSeries.map((series) => series.slug), "series slug");

  const seriesSlugs = new Set(blogSeries.map((series) => series.slug));
  const orderedParts: string[] = [];

  for (const post of blogPosts) {
    if (!post.series) continue;
    if (!seriesSlugs.has(post.series.slug)) {
      throw new Error(`Blog post ${post.slug} references unknown series ${post.series.slug}`);
    }
    if (!Number.isInteger(post.series.part) || post.series.part < 1) {
      throw new Error(`Blog post ${post.slug} has invalid series part ${post.series.part}`);
    }
    orderedParts.push(`${post.series.slug}:${post.series.part}`);
  }

  assertUnique(orderedParts, "series part");
}
