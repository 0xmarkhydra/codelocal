"use client";

import Link from "next/link";
import { useDashboardResource } from "../use-dashboard-resource";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";

type NeedsYouItem = {
  kind: string;
  title: string;
  goalId?: string;
  planId?: string;
  taskId?: string;
  detail?: string;
  priority: number;
};

type Goal = {
  goalId: string;
  projectId: string;
  title: string;
  status: string;
  updatedAt: number;
};

type ExecutiveSummary = {
  summary: { activeGoals: number; runningTasks: number; testingTasks: number; doneTasks: number; needsYou: number };
  needsYou: NeedsYouItem[];
  recentGoals: Goal[];
};

function isExecutiveSummary(value: unknown): value is ExecutiveSummary {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v.summary === "object" && Array.isArray(v.needsYou) && Array.isArray(v.recentGoals);
}

export function ExecutiveHome() {
  const { state, retry } = useDashboardResource("/api/v1/project-os/summary", isExecutiveSummary);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Executive Home"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const { summary, needsYou, recentGoals } = state.value;

  return (
    <div style={{ display: "grid", gap: 16 }}>
      <section aria-label="Global command">
        <h1 style={{ fontSize: 22, margin: 0 }}>Executive Home</h1>
        <p style={{ opacity: 0.75, margin: "6px 0 12px" }}>
          Một ô chat điều hành duy nhất phía trên cùng một Codex-like Project Chat. Số liệu bên dưới chỉ hiển thị
          trạng thái backend chứng minh được — không bịa số agent đang chạy.
        </p>
        <Link href="/dashboard" style={{ display: "inline-block", padding: "10px 16px", borderRadius: 10, border: "1px solid currentColor" }}>
          Mở Project Chat
        </Link>
      </section>

      <section aria-label="Summary" style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        {[
          ["Goals đang mở", summary.activeGoals],
          ["Tasks đang chạy", summary.runningTasks],
          ["Tasks chờ Tester", summary.testingTasks],
          ["Tasks DONE", summary.doneTasks],
          ["Needs You", summary.needsYou],
        ].map(([label, value]) => (
          <div key={String(label)} style={{ minWidth: 140, padding: 12, border: "1px solid rgba(127,127,127,.35)", borderRadius: 12 }}>
            <div style={{ fontSize: 24, fontWeight: 650 }}>{Number(value)}</div>
            <div style={{ opacity: 0.7, fontSize: 13 }}>{label}</div>
          </div>
        ))}
      </section>

      <section aria-label="Needs You">
        <h2 style={{ fontSize: 16 }}>Needs You</h2>
        {needsYou.length === 0 ? (
          <p style={{ opacity: 0.7 }}>Không có quyết định nào đang chờ bạn. Routine failures sẽ tự retry, không spam ở đây.</p>
        ) : (
          <ul style={{ display: "grid", gap: 8, padding: 0, listStyle: "none" }}>
            {needsYou.map((item, i) => (
              <li key={`${item.kind}-${item.goalId ?? ""}-${item.taskId ?? ""}-${i}`} style={{ padding: 10, border: "1px solid rgba(127,127,127,.35)", borderRadius: 10 }}>
                <strong>{item.title}</strong>
                <div style={{ opacity: 0.65, fontSize: 13 }}>{item.kind}{item.detail ? ` · ${item.detail}` : ""}</div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-label="Recent goals">
        <h2 style={{ fontSize: 16 }}>Goals gần đây</h2>
        {recentGoals.length === 0 ? (
          <p style={{ opacity: 0.7 }}>Chưa có Goal nào. Hãy mở Project Chat và nói ví dụ: “Làm payment mới cho BIDDI.”</p>
        ) : (
          <ul style={{ display: "grid", gap: 8, padding: 0, listStyle: "none" }}>
            {recentGoals.map((g) => (
              <li key={g.goalId} style={{ padding: 10, border: "1px solid rgba(127,127,127,.35)", borderRadius: 10 }}>
                <strong>{g.title}</strong>
                <div style={{ opacity: 0.65, fontSize: 13 }}>{g.status} · {g.projectId}</div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
