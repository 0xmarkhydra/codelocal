import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import styles from "./design.module.css";
import { PenpotDesignFrame } from "./penpot-design-frame";
import { getTranslations } from "@/lib/i18n/server";

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

export default async function DesignPage() {
  const t = await getTranslations();
  const designUrl = resolveDesignUrl();

  return (
    <section className={dashboard.content}>
      <div className={styles.page}>
        <h1 className="sr-only">{t("Design")}</h1>

        <div className={styles.embedShell}>
          <div className={styles.embedBar}>
            <span><i aria-hidden="true" />design.codelocal.cloud</span>
            <a href={designUrl} target="_blank" rel="noreferrer">
              {t("Open in new tab")} <AppIcon name="external" size={14} />
            </a>
          </div>
          <PenpotDesignFrame designUrl={designUrl} />
        </div>

        <p className={styles.embedFallback}>
          {t("Embedded workspace blocked?")} <a href={designUrl} target="_blank" rel="noreferrer">{t("Open Penpot in a new tab")}</a>. {t("Both use the same CodeLocal SSO.")}
        </p>
      </div>
    </section>
  );
}
