import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";
import { LiveOverview } from "../overview-live";
import { MCPEndpoint } from "./mcp-endpoint";

const connectionSteps = ["Thêm MCP server", "Chọn OAuth", "Scan tools", "Đăng nhập", "Cấp quyền"] as const;

export default function ConnectPage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}>
        <div>
          <span className={dashboard.eyebrow}>System</span>
          <h1>Connections</h1>
          <p>Mọi kết nối và trạng thái hệ thống của CodeLocal ở một nơi.</p>
        </div>
      </header>

      <LiveOverview />

      <details className={surface.setupDisclosure}>
        <summary>
          <span>
            <strong>Kết nối MCP</strong>
            <small>Endpoint, OAuth và hướng dẫn thiết lập</small>
          </span>
          <AppIcon name="chevron-down" size={16} />
        </summary>
        <div className={surface.grid}>
          <section className={`${surface.card} ${surface.span8}`}>
            <div className={surface.cardHeader}>
              <div><span className={dashboard.eyebrow}>Endpoint</span><h2 className={surface.title}>MCP server</h2></div>
              <span className={surface.statusBadge}><i /> OAuth ready</span>
            </div>
            <MCPEndpoint />
          </section>
          <section className={`${surface.card} ${surface.span4}`}>
            <span className={dashboard.eyebrow}>Setup</span>
            <h2 className={surface.title}>Kết nối trong 5 bước</h2>
            <ol className={surface.steps}>
              {connectionSteps.map((label, index) => <li className={surface.step} key={label}><b>{index + 1}</b><span>{label}</span></li>)}
            </ol>
          </section>
        </div>
      </details>
    </section>
  );
}
