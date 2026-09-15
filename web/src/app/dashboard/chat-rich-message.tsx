"use client";

import { isValidElement, useState, type ComponentPropsWithoutRef, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { AppIcon } from "./app-icon";
import { useTranslations } from "@/lib/i18n/provider";
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

const languageLabels: Record<string, string> = {
  ts: "TypeScript",
  tsx: "TSX",
  js: "JavaScript",
  jsx: "JSX",
  go: "Go",
  py: "Python",
  python: "Python",
  sh: "Shell",
  bash: "Shell",
  shell: "Shell",
  json: "JSON",
  yaml: "YAML",
  yml: "YAML",
  css: "CSS",
  html: "HTML",
  sql: "SQL",
  md: "Markdown",
  dockerfile: "Dockerfile",
  diff: "Diff",
  text: "Text",
};

function nodeText(node: ReactNode): string {
  if (node == null || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(nodeText).join("");
  if (isValidElement(node)) return nodeText((node.props as { children?: ReactNode }).children);
  return "";
}

function CodeBlock({ children }: { children?: ReactNode }) {
  const { t } = useTranslations();
  const [copied, setCopied] = useState(false);
  const codeElement = isValidElement(children) ? children : null;
  const codeProps = (codeElement?.props ?? {}) as { className?: string; children?: ReactNode };
  const language = /language-([\w-]+)/.exec(codeProps.className ?? "")?.[1];
  const source = nodeText(codeProps.children);

  async function copy() {
    try {
      await navigator.clipboard.writeText(source);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className={styles.richCodeWrap}>
      <div className={styles.richCodeHead}>
        <span className={styles.richCodeLang}>
          {language ? languageLabels[language] ?? language.toUpperCase() : "Code"}
        </span>
        <button
          className={styles.richCodeCopy}
          type="button"
          onClick={copy}
          data-copied={copied || undefined}
          aria-label={copied ? t("Code copied") : t("Copy code")}
        >
          <AppIcon name={copied ? "check" : "copy"} size={12} />
          {copied ? t("Copied") : t("Copy")}
        </button>
      </div>
      <pre className={styles.richCodeBlock}>{children}</pre>
    </div>
  );
}

function RichTable({ children }: { children?: ReactNode }) {
  const { t } = useTranslations();
  return (
    <div className={styles.richTableWrap} tabIndex={0} role="region" aria-label={t("Data table")}>
      <table>{children}</table>
    </div>
  );
}

const components: Components = {
  a: SafeLink,
  pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
  code: ({ className, children, ...props }) => (
    <code className={className} {...props}>{children}</code>
  ),
  table: RichTable,
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
