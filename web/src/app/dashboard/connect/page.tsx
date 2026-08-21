import Link from "next/link";
import { DashboardIcon } from "../dashboard-icon";
import visual from "../visual-dashboard.module.css";
import { MCPEndpoint } from "./mcp-endpoint";

export default function ConnectPage() {
  return (
    <section className={visual.page}>
      <header className={visual.head}>
        <div className={visual.headMeta}><h1>Connections</h1><span className={visual.online}>Online</span></div>
        <Link className={visual.iconButton} href="/#how-it-works" aria-label="Connection guide" title="Guide"><DashboardIcon name="book" /></Link>
      </header>
      <div className={visual.shell}>
        <section className={visual.panel}>
          <div className={visual.compactTop}><span className={visual.miniLabel}>MCP endpoint</span><DashboardIcon name="info" size={15} /></div>
          <MCPEndpoint />
        </section>
        <section className={`${visual.panel} ${visual.network}`} aria-label="AI clients connected to CodeLocal">
          {[0,1,2,3].map((i)=><span className={visual.beam} data-i={i} key={`b${i}`} />)}
          <div className={visual.hub}><DashboardIcon name="graph" size={34} /></div>
          <div className={visual.orbit} data-i="0" title="AI client"><DashboardIcon name="activity" /></div>
          <div className={visual.orbit} data-i="1" title="Desktop"><DashboardIcon name="monitor" /></div>
          <div className={visual.orbit} data-i="2" title="API"><DashboardIcon name="code" /></div>
          <div className={visual.orbit} data-i="3" title="CLI"><DashboardIcon name="bolt" /></div>
        </section>
        <section className={visual.panel}>
          <div className={visual.compactTop}><span className={visual.miniLabel}>Clients</span><span className={visual.online}>OAuth</span></div>
          <div className={visual.clientStrip}>{["AI","Claude","Cursor","VS Code","API","CLI"].map((label)=><div className={visual.clientTile} data-state="neutral" key={label}><span>{label}</span><i /></div>)}</div>
        </section>
        <section className={`${visual.panel} ${visual.quickFlow}`} aria-label="Quick connect flow">
          <span className={visual.flowStep}><DashboardIcon name="link" /></span><DashboardIcon name="arrow" size={14} />
          <span className={visual.flowStep}><DashboardIcon name="copy" /></span><DashboardIcon name="arrow" size={14} />
          <span className={visual.flowStep}><DashboardIcon name="plug" /></span><DashboardIcon name="arrow" size={14} />
          <span className={visual.flowStep}><DashboardIcon name="check" /></span>
        </section>
      </div>
    </section>
  );
}
