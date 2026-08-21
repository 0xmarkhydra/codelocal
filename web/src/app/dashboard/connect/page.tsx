import Link from "next/link";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";
import { MCPEndpoint } from "./mcp-endpoint";

function InfoTip({ label, children }: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <details className={surface.infoTip}>
      <summary aria-label={label} title={label}>i</summary>
      <div>{children}</div>
    </details>
  );
}

const connectionSteps = ["Add server", "Choose OAuth", "Sign in", "Approve"] as const;

export default function ConnectPage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}>
        <div>
          <h1>MCP Connections</h1>
          <p>Connect an AI client to this CodeLocal account.</p>
        </div>
        <div className={dashboard.headerActions}>
          <Link className={dashboard.headerAction} href="/#how-it-works">Guide</Link>
          <InfoTip label="About MCP connections">
            OAuth keeps access scoped to your signed-in CodeLocal account and authorized local runtime.
          </InfoTip>
        </div>
      </header>

      <div className={surface.grid}>
        <section className={`${surface.card} ${surface.span8}`}>
          <div className={surface.cardHeader}>
            <div className={surface.titleRow}>
              <h2 className={surface.title}>MCP server URL</h2>
              <InfoTip label="About the MCP server URL">
                Paste this endpoint into a compatible MCP client. OAuth metadata is discovered automatically.
              </InfoTip>
            </div>
            <span className={surface.statusBadge}><i /> OAuth ready</span>
          </div>
          <p className={surface.copy}>Use this endpoint in your MCP client.</p>
          <MCPEndpoint />
        </section>

        <section className={`${surface.card} ${surface.span4}`}>
          <div className={surface.titleRow}>
            <h2 className={surface.title}>Connect</h2>
            <InfoTip label="Connection flow">
              Follow these steps in the MCP client. CodeLocal asks for approval before the connection is authorized.
            </InfoTip>
          </div>
          <ol className={surface.steps}>
            {connectionSteps.map((label, index) => (
              <li className={surface.step} key={label}>
                <b>{index + 1}</b>
                <span>{label}</span>
              </li>
            ))}
          </ol>
        </section>
      </div>
    </section>
  );
}
