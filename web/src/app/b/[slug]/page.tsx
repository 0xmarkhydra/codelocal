import { notFound, permanentRedirect } from "next/navigation";
import { getBlogPostsForRender } from "@/lib/blog-server";
import { blogShortCode } from "@/lib/blog-short-link";

type ShortBlogLinkProps = {
  params: Promise<{ slug: string }>;
};

export default async function ShortBlogLink({ params }: ShortBlogLinkProps) {
  const { slug: code } = await params;
  const posts = await getBlogPostsForRender();
  const matches = posts.filter((post) => blogShortCode(post.slug) === code);
  if (matches.length !== 1) notFound();
  permanentRedirect(`/blogs/${encodeURIComponent(matches[0].slug)}`);
}
