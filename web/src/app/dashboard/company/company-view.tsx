"use client";

import { useSearchParams } from "next/navigation";
import { useDashboardResource } from "../use-dashboard-resource";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";

type CompanyResponse = {
  company: {
    goal: { goalId: string; title: string; status: string } | null;
    activeTasks: { taskId: string; title: string; status: string }[];
    waitingTasks: { taskId: string; title: string; status: string }[];
    tester: { reviewing: number; testing: number; done: number };
  };
  needsYou: { kind: string; title: string }[];
};

function isCompanyResponse(value: unknown): value is CompanyResponse {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  if (typeof v.company !== "object" || v.company === null) return false;
  return Array.isArray(v.needsYou);
}

export function CompanyView() {
  const params = useSearchParams();
  const projectId = (params.get("projectId") ?? "").trim();

  if (!projectId) {
    return (
      <div>
        <h1 style={{ fontSize: 22, margin: 0 }}>Company</h1>
        <p style={{ opacity: 0.7 }}>Company view đọc theo project. Hãy thêm ?projectId=... vào URL.</p>
      </div>
    );
  }
  return <CompanyViewInner projectId={projectId} />;
}

function CompanyViewInner({ projectId }: { projectId: string }) {
  const url = `/api/v1/projects/${encodeURIComponent(projectId)}/company`;
  const { state, retry } = useDashboardResource(url, isCompanyResponse);
  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        state={state}
        retry={retry}
        loadingMessage="Đang tải Company…"
        unauthenticatedMessage="Bạn cần đăng nhập để xem Company."
      />
    );
  }

  const { company, needsYou } = state.value;
  return (
    <div style={{ display: "grid", gap: 16 }}>
      <h1 style={{ fontSize: 22, margin: 0 }}>Company</h1>
      <p style={{ opacity: 0.7, margin: 0 }}>
        P0 dùng visualization cấu trúc đơn giản, không animation realtime giả. Mọi con số đều từ backend thật.
      </p>
      <section aria-label="Current goal" style={{ padding: 12, border: "1px solid rgba(127,127,127,.35)", borderRadius: 12 }}>
        <h2 style={{ fontSize: 16, margin: "0 0 8px" }}>Current Goal</h2>
        {company.goal ? (
          <div><strong>{company.goal.title}</strong><div style={{ opacity: 0.65, fontSize: 13 }}>{company.goal.status}</div></div>
        ) : (
          <div style={{ opacity: 0.7 }}>Chưa có Goal nào.</div>
        )}
      </section>
      <section aria-label="Chief status" style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        {[
          ["Active tasks", company.activeTasks.length],
          ["Waiting tasks", company.waitingTasks.length],
          ["Reviewing", company.tester.reviewing],
          ["Testing", company.tester.testing],
          ["Tester DONE", company.tester.done],
          ["Needs You", needsYou.length],
        ].map(([label, value]) => (
          <div key={String(label)} style={{ minWidth: 130, padding: 12, border: "1px solid rgba(127,127,127,.35)", borderRadius: 12 }}>
            <div style={{ fontSize: 22, fontWeight: 650 }}>{Number(value)}</div>
            <div style={{ opacity: 0.7, fontSize: 13 }}>{label}</div>
          </div>
        ))}
      </section>
      <section aria-label="Active workers">
        <h2 style={{ fontSize: 16 }}>Active workers / tasks</h2>
        {company.activeTasks.length === 0 ? (
          <p style={{ opacity: 0.7 }}>Không có task đang chạy.</p>
        ) : (
          <ul style={{ display: "grid", gap: 8, padding: 0, listStyle: "none" }}>
            {company.activeTasks.map((t) => (
              <li key={t.taskId} style={{ padding: 10, border: "1px solid rgba(127,127,127,.35)", borderRadius: 10 }}>
                <strong>{t.title}</strong>
                <div style={{ opacity: 0.65, fontSize: 13 }}>{t.status}</div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
