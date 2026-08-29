import { Fragment, type ReactNode } from "react";
import styles from "./dashboard-chat.module.css";

type Block =
  | { type: "paragraph"; lines: string[] }
  | { type: "heading"; level: number; text: string }
  | { type: "unordered"; items: string[] }
  | { type: "ordered"; items: string[] }
  | { type: "quote"; lines: string[] };

function parseBlocks(input: string) {
  const lines = input.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let paragraph: string[] = [];

  const flushParagraph = () => {
    if (!paragraph.length) return;
    blocks.push({ type: "paragraph", lines: paragraph });
    paragraph = [];
  };

  for (let index = 0; index < lines.length; index += 1) {
    const raw = lines[index];
    const line = raw.trim();

    if (!line) {
      flushParagraph();
      continue;
    }

    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      flushParagraph();
      blocks.push({ type: "heading", level: heading[1].length, text: heading[2] });
      continue;
    }

    if (/^[-*]\s+/.test(line)) {
      flushParagraph();
      const items: string[] = [];
      let cursor = index;
      while (cursor < lines.length) {
        const match = lines[cursor].trim().match(/^[-*]\s+(.+)$/);
        if (!match) break;
        items.push(match[1]);
        cursor += 1;
      }
      blocks.push({ type: "unordered", items });
      index = cursor - 1;
      continue;
    }

    if (/^\d+[.)]\s+/.test(line)) {
      flushParagraph();
      const items: string[] = [];
      let cursor = index;
      while (cursor < lines.length) {
        const match = lines[cursor].trim().match(/^\d+[.)]\s+(.+)$/);
        if (!match) break;
        items.push(match[1]);
        cursor += 1;
      }
      blocks.push({ type: "ordered", items });
      index = cursor - 1;
      continue;
    }

    if (line.startsWith(">")) {
      flushParagraph();
      const quoteLines: string[] = [];
      let cursor = index;
      while (cursor < lines.length && lines[cursor].trim().startsWith(">")) {
        quoteLines.push(lines[cursor].trim().replace(/^>\s?/, ""));
        cursor += 1;
      }
      blocks.push({ type: "quote", lines: quoteLines });
      index = cursor - 1;
      continue;
    }

    paragraph.push(raw.trim());
  }

  flushParagraph();
  return blocks;
}

function renderInline(text: string) {
  const tokenPattern = /(\*\*[^*]+\*\*|`[^`]+`|__[^_]+__)/g;
  const tokens = text.split(tokenPattern).filter(Boolean);

  return tokens.map<ReactNode>((token, index) => {
    if ((token.startsWith("**") && token.endsWith("**")) || (token.startsWith("__") && token.endsWith("__"))) {
      return <strong key={`${token}-${index}`}>{token.slice(2, -2)}</strong>;
    }
    if (token.startsWith("`") && token.endsWith("`")) {
      return <code key={`${token}-${index}`}>{token.slice(1, -1)}</code>;
    }
    return <Fragment key={`${token}-${index}`}>{token}</Fragment>;
  });
}

export function ChatRichMessage({ content }: { content: string }) {
  const blocks = parseBlocks(content);

  return (
    <div className={styles.richMessage}>
      {blocks.map((block, index) => {
        if (block.type === "heading") {
          const Heading = block.level <= 2 ? "h3" : "h4";
          return <Heading key={`heading-${index}`}>{renderInline(block.text)}</Heading>;
        }
        if (block.type === "unordered") {
          return (
            <ul key={`ul-${index}`} className={styles.richList}>
              {block.items.map((item, itemIndex) => <li key={`${item}-${itemIndex}`}>{renderInline(item)}</li>)}
            </ul>
          );
        }
        if (block.type === "ordered") {
          return (
            <ol key={`ol-${index}`} className={`${styles.richList} ${styles.richOrderedList}`}>
              {block.items.map((item, itemIndex) => <li key={`${item}-${itemIndex}`}><span>{itemIndex + 1}</span><div>{renderInline(item)}</div></li>)}
            </ol>
          );
        }
        if (block.type === "quote") {
          return <blockquote key={`quote-${index}`}>{block.lines.map((line, lineIndex) => <Fragment key={`${line}-${lineIndex}`}>{renderInline(line)}{lineIndex < block.lines.length - 1 ? <br /> : null}</Fragment>)}</blockquote>;
        }
        return <p key={`p-${index}`}>{block.lines.map((line, lineIndex) => <Fragment key={`${line}-${lineIndex}`}>{renderInline(line)}{lineIndex < block.lines.length - 1 ? <br /> : null}</Fragment>)}</p>;
      })}
    </div>
  );
}
