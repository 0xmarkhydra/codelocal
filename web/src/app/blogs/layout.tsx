import type { Metadata } from "next";
import { validateBlogRegistry } from "@/lib/blog-validation";
import { BlogFooter, BlogHeader } from "../blog/_components";
import styles from "../blog/blog.module.css";

validateBlogRegistry();

export const metadata: Metadata = {
  title: {
    default: "Blog",
    template: "%s · CodeLocal Blog",
  },
  description:
    "Practical notes from CodeLocal and the community on local-first AI infrastructure, coding agents, MCP, security and durable project intelligence.",
  openGraph: {
    title: "CodeLocal Blog",
    description:
      "Practical notes on local-first AI infrastructure, coding agents, MCP, security and durable project intelligence.",
    type: "website",
  },
};

export default function BlogsLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className={styles.blogShell}>
      <BlogHeader />
      {children}
      <BlogFooter />
    </div>
  );
}
