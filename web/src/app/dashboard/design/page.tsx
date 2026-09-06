import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import styles from "./design.module.css";
import { PenpotDesignFrame } from "./penpot-design-frame";

const DEFAULT_DESIGN_URL = "https://design.codelocal.cloud";

function resolveDesignUrl() {
  const raw = process.env.CODELOCAL_DESIGN_URL?.trim() || DEFAULT_DESIGN_URL;
  try {
    const url = new URL(raw);
    const localHTTP = url.protocol === "http:" && ["localhost", "127.0.0.1", "::1"].includes(url.hostname);
    if (url.protocol !== "https:" && !localHTTP) return DEFAULT_DESIGN_URL;
    return url.toString().replace(/\/$/, "");
  } catch {
    return DEFAULT_DESIGN_URL;
  }
}

export default function DesignPage() {
  const designUrl = resolveDesignUrl();

  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <header className={styles.hero}>
          <div className={styles.heroIcon} aria-hidden="true"><AppIcon name="edit" size={24} /></div>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>System Plugin · Penpot 2.17</span>
            <h1>Design</h1>
            <p>Thiết kế trong Penpot bằng chính CodeLocal identity, rồi dùng managed MCP để tạo hoặc chỉnh sửa design từ agent.</p>
          </div>
          <span className={styles.status} data-active="true"><i aria-hidden="true" />SSO + Managed MCP</span>
        </header>

        <div className={styles.embedShell}>
          <div className={styles.embedBar}>
            <span><i aria-hidden="true" />design.codelocal.cloud</span>
            <a href={designUrl} target="_blank" rel="noreferrer">
              Open in new tab <AppIcon name="external" size={14} />
            </a>
          </div>
          <PenpotDesignFrame designUrl={designUrl} />
        </div>

        <p className={styles.embedFallback}>
          Nếu trình duyệt chặn embedded workspace, <a href={designUrl} target="_blank" rel="noreferrer">mở Penpot trong tab riêng</a>. Cả hai đường dẫn dùng cùng CodeLocal SSO.
        </p>
      </div>
    </section>
  );
}
