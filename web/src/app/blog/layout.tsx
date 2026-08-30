import type { Metadata } from "next";
import { validateBlogRegistry } from "@/lib/blog-validation";
import { BlogFooter, BlogHeader } from "./_components";
import styles from "./blog.module.css";

validateBlogRegistry();

export const metadata: Metadata = {
  title: {
    default: "Blog",
    template: "%s · CodeLocal Blog",
  },
  description:
    "Practical notes from CodeLocal on local-first AI infrastructure, coding agents, MCP, security and durable project intelligence.",
  openGraph: {
    title: "CodeLocal Blog",
    description:
      "Practical notes on local-first AI infrastructure, coding agents, MCP, security and durable project intelligence.",
    type: "website",
  },
};

export default function BlogLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className={styles.blogShell}>
      <BlogHeader />
      {children}
      <BlogFooter />
    </div>
  );
}
