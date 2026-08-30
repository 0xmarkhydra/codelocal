import { BlogEditor } from "../blog-editor";

export default async function BlogEditorPage({ params }: { params: Promise<{ postID: string }> }) {
  const { postID } = await params;
  return <BlogEditor postID={postID} />;
}
