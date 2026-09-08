"use client";

import Link from "next/link";
import { isDashboardOverview } from "@/lib/contracts/dashboard";
import { AppIcon, type AppIconName } from "./app-icon";
import { DashboardResourceFeedback } from "./dashboard-resource-feedback";
import styles from "./overview.module.css";
import { useDashboardResource } from "./use-dashboard-resource";

const number = new Intl.NumberFormat("vi-VN", {
  notation: "compact",
  maximumFractionDigits: 1,
});
const destinations: {
  icon: AppIconName;
  title: string;
  description: string;
  href: string;
}[] = [
  {
    icon: "connection",
    title: "Kết nối AI",
    description: "Thiết lập MCP cho client của bạn",
    href: "/dashboard/connect",
  },
  {
    icon: "skill",
    title: "Khám phá Skills",
    description: "Khả năng mở rộng cho AI",
    href: "/dashboard/skills",
  },
  {
    icon: "shield",
    title: "Kiểm tra bảo mật",
    description: "Quản lý phiên và quyền truy cập",
    href: "/dashboard/security",
  },
];

export function LiveOverview() {
  const { state, retry } = useDashboardResource(
    "/api/v1/dashboard/overview",
    isDashboardOverview,
  );
  if (state.kind !== "ready")
    return (
      <DashboardResourceFeedback
        label="tổng quan workspace"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  const overview = state.value;
  return (
    <div className={styles.overview}>
      <section className={styles.metrics} aria-label="Số liệu tài khoản">
        <Metric
          icon="folder"
          label="Dự án"
          value={number.format(overview.workspaces.total)}
          detail={`${overview.workspaces.active} đang hoạt động`}
          tone="purple"
          href="/dashboard/workspaces"
        />
        <Metric
          icon="device"
          label="Thiết bị online"
          value={number.format(overview.devices.online)}
          detail={`${overview.devices.paired} thiết bị đã ghép nối`}
          tone="cyan"
          href="/dashboard/devices"
        />
        <Metric
          icon="connection"
          label="MCP calls / 24 giờ"
          value={
            overview.usage.available
              ? number.format(overview.usage.last24h.calls)
              : "—"
          }
          detail={
            overview.usage.available
              ? "Lượt gọi được ghi nhận"
              : "Chưa có dữ liệu sử dụng"
          }
          tone="orange"
          href="/dashboard/usage"
        />
        <Metric
          icon="usage"
          label="Tokens / 24 giờ"
          value={
            overview.usage.available
              ? number.format(overview.usage.last24h.totalTokensEstimated)
              : "—"
          }
          detail={
            overview.usage.available
              ? "Ước tính từ hoạt động MCP"
              : "Chưa có dữ liệu sử dụng"
          }
          tone="pink"
          href="/dashboard/usage"
        />
      </section>
      <div className={styles.workspaceLayout}>
        <section className={styles.projects}>
          <header className={styles.panelHead}>
            <div>
              <h2>Dự án gần đây</h2>
              <span>Workspace được kết nối với tài khoản</span>
            </div>
            <Link href="/dashboard/workspaces">
              Xem tất cả <span aria-hidden="true">↗</span>
            </Link>
          </header>
          {overview.workspaces.recent.length === 0 ? (
            <div className={styles.empty}>
              <AppIcon name="folder" size={30} />
              <strong>Không gian cho dự án đầu tiên.</strong>
              <p>
                Chạy <code>codelocal .</code> trong thư mục dự án để kết nối
                workspace của bạn.
              </p>
              <Link href="/dashboard/connect">
                Hướng dẫn kết nối <span aria-hidden="true">↗</span>
              </Link>
            </div>
          ) : (
            <div className={styles.workspaceList}>
              {overview.workspaces.recent.slice(0, 4).map((workspace) => (
                <Link
                  className={styles.workspaceRow}
                  href={`/dashboard/code-graph?deviceId=${encodeURIComponent(workspace.deviceId)}&workspaceId=${encodeURIComponent(workspace.workspaceId)}`}
                  key={`${workspace.deviceId}:${workspace.workspaceId}`}
                >
                  <span className={styles.folderIcon}>
                    <AppIcon name="folder" size={20} />
                  </span>
                  <div>
                    <strong>{workspace.workspaceName}</strong>
                    <small>{workspace.deviceName}</small>
                  </div>
                  <span
                    className={styles.workspaceState}
                    data-state={workspace.status}
                  >
                    <i />
                    {workspace.status === "active"
                      ? "Online"
                      : workspace.status === "sleeping"
                        ? "Đang nghỉ"
                        : "Offline"}
                  </span>
                  <AppIcon name="chevron-right" size={14} />
                </Link>
              ))}
            </div>
          )}
          <div className={styles.projectsFoot}>
            <AppIcon name="shield" size={13} /> Chỉ hiển thị workspace bạn được
            phép truy cập.
          </div>
        </section>
        <section className={styles.quickAccess}>
          <header className={styles.panelHead}>
            <div>
              <h2>Bước tiếp theo</h2>
              <span>Thiết lập không gian làm việc</span>
            </div>
            <AppIcon name="target" size={18} />
          </header>
          <div className={styles.destinations}>
            {destinations.map((item) => (
              <Link href={item.href} key={item.href}>
                <span className={styles.destinationIcon}>
                  <AppIcon name={item.icon} size={18} />
                </span>
                <div>
                  <strong>{item.title}</strong>
                  <small>{item.description}</small>
                </div>
                <AppIcon name="chevron-right" size={14} />
              </Link>
            ))}
          </div>
        </section>
      </div>
      <div className={styles.dataNote}>
        <span>Dữ liệu từ lần tải gần nhất · MCP tokens là số ước tính</span>
        <button type="button" onClick={retry}>
          <AppIcon name="refresh" size={13} /> Làm mới
        </button>
      </div>
    </div>
  );
}

function Metric({
  icon,
  label,
  value,
  detail,
  tone,
  href,
}: {
  icon: AppIconName;
  label: string;
  value: string;
  detail: string;
  tone: string;
  href: string;
}) {
  return (
    <Link className={styles.metric} data-tone={tone} href={href}>
      <span className={styles.metricLabel}>
        <span>{label}</span>
        <AppIcon name={icon} size={17} />
      </span>
      <strong>{value}</strong>
      <span className={styles.metricDetail}>
        {detail}
        <span aria-hidden="true">↗</span>
      </span>
    </Link>
  );
}
