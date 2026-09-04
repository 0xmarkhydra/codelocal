"use client";

import { useSearchParams } from "next/navigation";
import { useDashboardResource } from "../use-dashboard-resource";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";

type Task = { taskId: string; title: string; status: string; taskKind: string; updatedAt: number };
type Goal = { goalId: string; title: string; status: string };

type WorkResponse = { tasks: Task[]; goals: Goal[]; needsYou: { kind: string; title: string; taskId?: string }[] };

function isWorkResponse(value: unknown): value is WorkResponse {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return Array.isArray(v.tasks) && Array.isArray(v.goals) && Array.isArray(v.needsYou);
}

const GROUPS: [string, string[]][] = [
  ["Goals", []],
  ["Running", ["READY", "ASSIGNED", "RUNNING"]],
  ["Testing", ["REVIEWING", "TESTING"]],
  ["Done", ["DONE"]],
  ["Needs attention", ["BLOCKED", "WAITING_HUMAN", "WAITING_EXTERNAL", "FAILED", "RETRYING"]],
];

export function WorkBoard() {
  const params = useSearchParams();
  const projectId = (params.get("projectId") ?? "").trim();

  if (!projectId) {
    return (
      <div>
        <h1 style={{ fontSize: 22, margin: 0 }}>Work</h1>
        <p style={{ opacity: 0.7 }}>
          Work view đọc theo project. Hãy mở Executive Home rồi vào một project, hoặc thêm ?projectId=... vào URL.
        </p>
      </div>
    );
  }
  return <WorkBoardInner projectId={projectId} />;
}

function WorkBoardInner({ projectId }: { projectId: string }) {
  const url = `/api/v1/projects/${encodeURIComponent(projectId)}/work`;
  const { state, retry } = useDashboardResource(url, isWorkResponse);
  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Work"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const byStatus = new Map<string, Task[]>();
  for (const t of state.value.tasks) {
    const list = byStatus.get(t.status) ?? [];
    list.push(t);
    byStatus.set(t.status, list);
  }

  return (
    <div style={{ display: "grid", gap: 16 }}>
      <h1 style={{ fontSize: 22, margin: 0 }}>Work</h1>
      {GROUPS.map(([label, statuses]) => {
        if (label === "Goals") {
          return (
            <section key={label} aria-label={label}>
              <h2 style={{ fontSize: 16 }}>{label} ({state.value.goals.length})</h2>
              {state.value.goals.length === 0 ? (
                <p style={{ opacity: 0.7 }}>Chưa có Goal nào trong project này.</p>
              ) : (
                <ul style={{ display: "grid", gap: 8, padding: 0, listStyle: "none" }}>
                  {state.value.goals.map((g) => (
                    <li key={g.goalId} style={{ padding: 10, border: "1px solid rgba(127,127,127,.35)", borderRadius: 10 }}>
                      <strong>{g.title}</strong>
                      <div style={{ opacity: 0.65, fontSize: 13 }}>{g.status}</div>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          );
        }
        const items = statuses.flatMap((s) => byStatus.get(s) ?? []);
        return (
          <section key={label} aria-label={label}>
            <h2 style={{ fontSize: 16 }}>{label} ({items.length})</h2>
            {items.length === 0 ? (
              <p style={{ opacity: 0.6 }}>Trống.</p>
            ) : (
              <ul style={{ display: "grid", gap: 8, padding: 0, listStyle: "none" }}>
                {items.map((t) => (
                  <li key={t.taskId} style={{ padding: 10, border: "1px solid rgba(127,127,127,.35)", borderRadius: 10 }}>
                    <strong>{t.title}</strong>
                    <div style={{ opacity: 0.65, fontSize: 13 }}>{t.status} · {t.taskKind}</div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        );
      })}
    </div>
  );
}
