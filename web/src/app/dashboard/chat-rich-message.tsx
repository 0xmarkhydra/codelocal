import type { ComponentPropsWithoutRef } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import styles from "./dashboard-chat.module.css";

function SafeLink({ href = "", children, ...props }: ComponentPropsWithoutRef<"a">) {
  const external = /^https?:\/\//i.test(href);
  return (
    <a
      {...props}
      href={href}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
    >
      {children}
    </a>
  );
}

const components: Components = {
  a: SafeLink,
  pre: ({ children }) => <pre className={styles.richCodeBlock}>{children}</pre>,
  code: ({ className, children, ...props }) => (
    <code className={className} {...props}>{children}</code>
  ),
  table: ({ children }) => (
    <div className={styles.richTableWrap} tabIndex={0} role="region" aria-label="Bảng dữ liệu">
      <table>{children}</table>
    </div>
  ),
};

export function ChatRichMessage({ content }: { content: string }) {
  return (
    <div className={styles.richMessage}>
      <Markdown remarkPlugins={[remarkGfm]} components={components} skipHtml>
        {content}
      </Markdown>
    </div>
  );
}
