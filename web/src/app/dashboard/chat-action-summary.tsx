"use client";

import { AppIcon } from "./app-icon";
import { PluginApproval } from "./plugin-approval";
import styles from "./chat-action-summary.module.css";

export type ChatToolCall = {
  id: string;
  name: string;
  arguments: string;
  result?: string;
  durationMs?: number;
  status: "running" | "done" | "error" | "approval_required";
};

function actionLabel(name: string, status: ChatToolCall["status"]) {
  const labels: Record<string, [string, string, string]> = {
    list_workspaces: ["Đang xem workspaces", "Đã xem workspaces", "Xem workspaces"],
    list_devices: ["Đang kiểm tra thiết bị", "Đã kiểm tra thiết bị", "Kiểm tra thiết bị"],
    search_project_brain: ["Đang truy vấn Brain", "Đã truy vấn Brain", "Truy vấn Brain"],
    get_workspace_detail: ["Đang đọc dự án", "Đã đọc dự án", "Đọc dự án"],
    list_project_files: ["Đang xem mã nguồn", "Đã xem mã nguồn", "Xem mã nguồn"],
    read_project_file: ["Đang đọc file", "Đã đọc file", "Đọc file"],
    search_project_code: ["Đang tìm trong code", "Đã tìm trong code", "Tìm trong code"],
    edit_project_file: ["Đang sửa code", "Đã sửa code", "Sửa code"],
    write_project_file: ["Đang cập nhật file", "Đã cập nhật file", "Cập nhật file"],
    apply_project_patch: ["Đang áp dụng thay đổi", "Đã áp dụng thay đổi", "Áp dụng thay đổi"],
    run_project_command: ["Đang chạy lệnh", "Đã chạy lệnh", "Chạy lệnh"],
    poll_project_command: ["Đang chờ lệnh", "Lệnh đã kết thúc", "Chờ lệnh"],
    verify_project_changes: ["Đang kiểm tra thay đổi", "Đã kiểm tra thay đổi", "Kiểm tra thay đổi"],
  };
  const label = labels[name];
  if (!label) return name.replaceAll("_", " ");
  if (status === "running") return label[0];
  if (status === "done") return label[1];
  return label[2];
}

function durationLabel(durationMs: number) {
  if (durationMs < 1000) return `${durationMs}ms`;
  if (durationMs < 60_000) return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)}s`;
  const minutes = Math.floor(durationMs / 60_000);
  const seconds = Math.round((durationMs % 60_000) / 1000);
  return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

function summaryState(actions: ChatToolCall[]) {
  if (actions.some((action) => action.status === "approval_required")) return "approval";
  if (actions.some((action) => action.status === "running")) return "running";
  if (actions.some((action) => action.status === "error")) return "error";
  return "done";
}

function summaryLabel(actions: ChatToolCall[], state: ReturnType<typeof summaryState>) {
  if (state === "approval") return "Đang chờ cấp quyền";
  if (state === "running") return `${actions.filter((action) => action.status === "running").length} bước đang chạy`;
  if (state === "error") {
    const failed = actions.filter((action) => action.status === "error").length;
    return `${actions.length - failed} hoàn tất · ${failed} lỗi`;
  }
  return `${actions.length} bước đã hoàn tất`;
}

export function ChatActionSummary({ actions, busy, onApprove }: { actions: ChatToolCall[]; busy: boolean; onApprove: () => void }) {
  const state = summaryState(actions);
  const durationMs = actions.reduce((total, action) => total + Math.max(0, action.durationMs || 0), 0);
  const approvals = actions.filter((action) => action.status === "approval_required");
  const stateLabel = state === "approval" ? "Cần cấp quyền" : state === "error" ? "Hoàn tất, có lỗi" : state === "running" ? "Đang chạy" : "Hoàn tất";
  const label = summaryLabel(actions, state);

  return (
    <div className={styles.actionGroup} data-state={state}>
      <details className={styles.disclosure}>
        <summary role="button" aria-label={`${label}${durationMs > 0 ? `, ${durationLabel(durationMs)}` : ""}`}>
          <span className={styles.stateIcon} aria-hidden="true">{state === "done" ? <AppIcon name="check" size={13} /> : <i />}</span>
          <span className={styles.summaryCopy} aria-live="polite">
            <strong>{label}</strong>
            <small>{stateLabel}{durationMs > 0 ? ` · ${durationLabel(durationMs)}` : ""}</small>
          </span>
          <AppIcon className={styles.chevron} name="chevron-right" size={14} />
        </summary>
        <ol className={styles.timeline}>
          {actions.map((action) => (
            <li key={action.id} data-state={action.status}>
              <span className={styles.timelineDot} aria-hidden="true" />
              <div className={styles.actionCopy}>
                <strong>{actionLabel(action.name, action.status)}</strong>
                <small>{action.status === "approval_required" ? "Cần quyền" : action.status === "error" ? "Lỗi" : action.status === "running" ? "Đang chạy" : "Hoàn tất"}{action.durationMs ? ` · ${durationLabel(action.durationMs)}` : ""}</small>
                {action.arguments || action.result ? (
                  <details className={styles.actionData}>
                    <summary>Chi tiết</summary>
                    {action.arguments ? <code>{action.arguments}</code> : null}
                    {action.result ? <code>{action.result.length > 800 ? `${action.result.slice(0, 800)}…` : action.result}</code> : null}
                  </details>
                ) : null}
              </div>
            </li>
          ))}
        </ol>
      </details>
      {approvals.map((action) => {
        if (action.name === "call_plugin_tool") {
          let request: { approvalId?: unknown; plugin?: unknown; connection?: unknown; tool?: unknown; arguments?: unknown } = {};
          try { request = JSON.parse(action.result || "{}"); } catch { /* Missing approval data must not enable execution. */ }
          if (typeof request.approvalId !== "string" || !/^[a-f0-9]{48}$/.test(request.approvalId)) return null;
          return <PluginApproval key={`${action.id}-approval`} id={request.approvalId} busy={busy} />;
        }
        return <div className={styles.approvalRow} key={`${action.id}-approval`}>
          <span><AppIcon name="shield" size={15} /><strong>{actionLabel(action.name, action.status)}</strong><small>Cần quyền để tiếp tục</small></span>
          <button type="button" onClick={onApprove} disabled={busy}>Toàn quyền truy cập</button>
        </div>;
      })}
    </div>
  );
}
