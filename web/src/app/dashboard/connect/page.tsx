import Link from "next/link";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";
import { MCPEndpoint } from "./mcp-endpoint";

export default function ConnectPage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}>
        <div><span className={dashboard.eyebrow}>Secure MCP connection</span><h1>MCP Connections</h1><p>Connect compatible AI clients to the same authenticated CodeLocal account and authorized local runtime.</p></div>
        <Link className={dashboard.productionLink} href="/#setup">Setup guide</Link>
      </header>
      <div className={surface.grid}>
        <section className={`${surface.card} ${surface.span8}`}>
          <span className={dashboard.eyebrow}>OAuth discovery enabled</span>
          <h2 className={surface.title}>MCP server URL</h2>
          <p className={surface.copy}>Add this endpoint to any compatible remote MCP client. OAuth metadata is discovered automatically and authorization remains scoped to this CodeLocal account.</p>
          <MCPEndpoint />
        </section>
        <section className={`${surface.card} ${surface.span4}`}>
          <span className={dashboard.eyebrow}>Connection flow</span>
          <h2 className={surface.title}>Four steps</h2>
          <div className={surface.steps}>
            {[["1","Add MCP server"],["2","Choose OAuth"],["3","Sign in to CodeLocal"],["4","Approve connection"]].map(([n,label]) => <div className={surface.step} key={n}><b>{n}</b><span>{label}</span></div>)}
          </div>
        </section>
      </div>
    </section>
  );
}
