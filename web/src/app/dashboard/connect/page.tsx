import Image from "next/image";
import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";
import { MCPEndpoint } from "./mcp-endpoint";
import { getTranslations } from "@/lib/i18n/server";

const connectionSteps = ["Add MCP server", "Choose OAuth", "Scan tools", "Sign in", "Authorize access"] as const;

export default async function ConnectPage() {
  const t = await getTranslations();
  return (
    <section className={dashboard.content}>
      <h1 className="sr-only">{t("MCP connections")}</h1>
      <details className={surface.setupDisclosure} open>
        <summary>
          <span>
            <strong>{t("MCP connections")}</strong>
            <small>{t("Endpoint, OAuth and setup")}</small>
          </span>
          <AppIcon name="chevron-down" size={16} />
        </summary>
        <div className={surface.grid}>
          <section className={`${surface.card} ${surface.span8}`}>
            <div className={surface.cardHeader}>
              <div className={surface.endpointHeading}>
                <Image className={surface.endpointLogo} src="/codelocal-icon.png" alt="" width={44} height={44} priority />
                <div><span className={dashboard.eyebrow}>{t("Endpoint")}</span><h2 className={surface.title}>{t("MCP server")}</h2></div>
              </div>
              <div className={surface.endpointActions}>
                <a className={surface.iconDownload} href="/codelocal-icon.png" download="codelocal-icon.png">
                  <AppIcon name="download" size={15} />
                  <span>{t("Download")}</span>
                </a>
                <span className={surface.statusBadge}><i /> {t("OAuth ready")}</span>
              </div>
            </div>
            <MCPEndpoint />
          </section>
          <section className={`${surface.card} ${surface.span4}`}>
            <span className={dashboard.eyebrow}>{t("Setup")}</span>
            <h2 className={surface.title}>{t("Connect in five steps")}</h2>
            <ol className={surface.steps}>
              {connectionSteps.map((label, index) => <li className={surface.step} key={label}><b>{index + 1}</b><span>{t(label)}</span></li>)}
            </ol>
          </section>
        </div>
      </details>
    </section>
  );
}
