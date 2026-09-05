import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import styles from "./design.module.css";

const DEFAULT_DESIGN_URL = "https://design.penpot.app";

function resolveDesignUrl() {
  const raw = process.env.CODELOCAL_DESIGN_URL?.trim() || DEFAULT_DESIGN_URL;
  try {
    const url = new URL(raw);
    if (url.protocol !== "https:" && url.protocol !== "http:") return DEFAULT_DESIGN_URL;
    return url.toString().replace(/\/$/, "");
  } catch {
    return DEFAULT_DESIGN_URL;
  }
}

export default function DesignPage() {
  const designUrl = resolveDesignUrl();
  const mcpConfigured = Boolean(process.env.CODELOCAL_PENPOT_MCP_URL?.trim());
  const selfHosted = designUrl !== DEFAULT_DESIGN_URL;

  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.hero}>
          <div className={styles.heroIcon} aria-hidden="true"><AppIcon name="edit" size={24} /></div>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>Design engine</span>
            <h1>Design</h1>
            <p>Dựng giao diện trong Penpot rồi đưa thiết kế trở lại workflow CodeLocal để tiếp tục code và review.</p>
          </div>
          <span className={styles.status} data-active={mcpConfigured ? "true" : "false"}>
            <i aria-hidden="true" />{mcpConfigured ? "MCP ready" : "Design ready"}
          </span>
        </header>

        <div className={styles.workspace}>
          <div className={styles.preview}>
            <div className={styles.previewChrome}>
              <span /><span /><span />
              <div>{selfHosted ? "CodeLocal Penpot" : "Penpot"}</div>
            </div>
            <div className={styles.canvas}>
              <div className={styles.leftRail}>
                <i /><i /><i /><i /><i />
              </div>
              <div className={styles.frame}>
                <div className={styles.frameHeader}><span /><span /></div>
                <div className={styles.frameBody}>
                  <div className={styles.frameTitle} />
                  <div className={styles.frameText} />
                  <div className={styles.frameGrid}><i /><i /><i /></div>
                </div>
              </div>
              <div className={styles.rightRail}><i /><i /><i /><i /></div>
            </div>
          </div>

          <aside className={styles.panel}>
            <div>
              <span className={styles.eyebrow}>Penpot workspace</span>
              <h2>Dựng UI ngay bây giờ</h2>
              <p>Penpot chạy ở tab riêng để tránh giới hạn frame của browser. Khi Railway self-host được gắn domain, CodeLocal chỉ cần đổi một biến môi trường.</p>
            </div>
            <a className={styles.primaryAction} href={designUrl} target="_blank" rel="noreferrer">
              <span>Open Penpot</span><AppIcon name="external" size={16} />
            </a>
            <div className={styles.meta}>
              <div><span>Runtime</span><strong>{selfHosted ? "Self-hosted" : "Penpot Cloud"}</strong></div>
              <div><span>AI bridge</span><strong>{mcpConfigured ? "Configured" : "Ready to connect"}</strong></div>
              <div><span>Workflow</span><strong>Design → CodeLocal → Code</strong></div>
            </div>
          </aside>
        </div>

        <div className={styles.cards}>
          <article><span className={styles.cardIcon}><AppIcon name="edit" size={18} /></span><div><h3>Design visually</h3><p>Frames, components, tokens, variants và prototype trong Penpot.</p></div></article>
          <article><span className={styles.cardIcon}><AppIcon name="connection" size={18} /></span><div><h3>AI bridge ready</h3><p>Server-side MCP config đã có chỗ để nối agent với canvas mà không lộ token ra browser.</p></div></article>
          <article><span className={styles.cardIcon}><AppIcon name="code" size={18} /></span><div><h3>Ship to code</h3><p>Boundary này giữ Design tách khỏi runtime code, nên sau có thể sync tokens/components mà không thay lại UI.</p></div></article>
        </div>
      </div>
    </section>
  );
}
