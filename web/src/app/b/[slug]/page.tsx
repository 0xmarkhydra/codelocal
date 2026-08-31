import { permanentRedirect } from "next/navigation";

type ShortBlogLinkProps = {
  params: Promise<{ slug: string }>;
};

export default async function ShortBlogLink({ params }: ShortBlogLinkProps) {
  const { slug } = await params;
  permanentRedirect(`/blogs/${encodeURIComponent(slug)}`);
}
