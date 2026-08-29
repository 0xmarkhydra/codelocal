"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { AppIcon } from "../app-icon";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";

type User = {
  id: string; email: string; referralCode: string; referredByCode: string; createdAt: number; inviteCount: number;
  lastDeviceSeenAt: number; lastMcpUsedAt: number; runtimeActive: boolean; mcpActive: boolean;
};
type Admin = { totalUsers: number; activeUsers: number; runtimeOnline: number; usingMcpNow: number; users: User[] };

function isAdmin(value: unknown): value is Admin {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return typeof v.totalUsers === "number" && typeof v.activeUsers === "number" && typeof v.runtimeOnline === "number" && typeof v.usingMcpNow === "number" && Array.isArray(v.users);
}
function status(user: User) { return user.mcpActive ? "Đang dùng MCP" : user.runtimeActive ? "Runtime online" : "Offline"; }
function when(value: number) { return value > 0 ? new Date(value).toLocaleString() : "Chưa có"; }

export function AdminLive() {
  const router = useRouter();
  const [data, setData] = useState<Admin | null>(null);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);

  useEffect(() => {
    fetch("/api/v1/admin", { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (response.status === 401) { router.replace(`/login?next=${encodeURIComponent("/dashboard/admin")}`); throw new Error("unauthorized"); }
        if (response.status === 403) throw new Error("forbidden");
        if (!response.ok) throw new Error("unavailable");
        return response.json() as Promise<unknown>;
      })
      .then((payload) => { if (!isAdmin(payload)) throw new Error("invalid"); setData(payload); })
      .catch((reason) => { if (reason instanceof Error && reason.message !== "unauthorized") setError(reason.message); });
  }, [router]);

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = query.trim().toLowerCase();
    if (!q) return data.users;
    return data.users.filter((user) => `${user.email} ${user.referralCode} ${user.referredByCode}`.toLowerCase().includes(q));
  }, [data, query]);
  const totalPages = Math.max(1, Math.ceil(filtered.length / 12));
  const safePage = Math.min(page, totalPages);
  const visible = filtered.slice((safePage - 1) * 12, safePage * 12);

  if (error === "forbidden") return <DashboardResourceFeedback kind="error" label="Admin" message="Tài khoản hiện tại không có quyền quản trị" onRetry={() => window.location.reload()} />;
  if (error) return <DashboardResourceFeedback kind="error" label="Admin" message="Admin dashboard tạm thời không khả dụng" onRetry={() => window.location.reload()} />;
  if (!data) return <DashboardResourceFeedback kind="loading" label="Admin" />;

  return (
    <div className={surface.adminLayout}>
      <section className={surface.adminMetrics} aria-label="Admin summary">
        <article><span>Users</span><strong>{data.totalUsers}</strong><small>{data.activeUsers} active</small></article>
        <article><span>Runtime online</span><strong>{data.runtimeOnline}</strong><small>thiết bị đang hoạt động</small></article>
        <article><span>MCP now</span><strong>{data.usingMcpNow}</strong><small>đang dùng ngay lúc này</small></article>
      </section>

      <section className={surface.adminUsersPanel}>
        <div className={surface.adminUsersHead}>
          <div><span className={dashboard.eyebrow}>Users</span><h2>Người dùng CodeLocal</h2></div>
          <label className={surface.adminSearch}>
            <AppIcon name="search" size={17} />
            <input type="search" value={query} onChange={(event) => { setQuery(event.target.value); setPage(1); }} placeholder="Tìm email hoặc referral code" />
          </label>
        </div>

        <div className={surface.adminTableModern} role="table" aria-label="Users">
          <div className={surface.adminTableHeader} role="row">
            <span>User</span><span>Status</span><span>Referral</span><span>Activity</span>
          </div>
          {visible.map((user) => (
            <div className={surface.adminTableRow} role="row" key={user.id}>
              <div><span className={surface.userAvatar} aria-hidden="true">{user.email.slice(0, 1).toUpperCase()}</span><span><strong>{user.email}</strong><small>Joined {when(user.createdAt)}</small></span></div>
              <div><span className={surface.adminState} data-active={user.mcpActive || user.runtimeActive || undefined}><i />{status(user)}</span></div>
              <div><strong>{user.referralCode}</strong><small>{user.referredByCode ? `Invited by ${user.referredByCode}` : "Direct account"} · {user.inviteCount} invites</small></div>
              <div><strong>MCP {when(user.lastMcpUsedAt)}</strong><small>Device {when(user.lastDeviceSeenAt)}</small></div>
            </div>
          ))}
        </div>

        <div className={surface.paginationBar}>
          <button type="button" disabled={safePage <= 1} aria-label="Previous page" onClick={() => setPage((p) => Math.max(1, p - 1))}><AppIcon name="chevron-left" size={14} /></button>
          <span>{safePage} / {totalPages}</span>
          <button type="button" disabled={safePage >= totalPages} aria-label="Next page" onClick={() => setPage((p) => Math.min(totalPages, p + 1))}><AppIcon name="chevron-right" size={14} /></button>
        </div>
      </section>

      <ReferralTree users={data.users} />
    </div>
  );
}

function ReferralTree({ users }: { users: User[] }) {
  const children = new Map<string, User[]>();
  const byCode = new Map<string, User>();
  for (const user of users) {
    byCode.set(user.referralCode.toUpperCase(), user);
    const parent = user.referredByCode.toUpperCase();
    const bucket = children.get(parent) ?? [];
    bucket.push(user);
    children.set(parent, bucket);
  }
  const roots: User[] = [];
  const root = byCode.get("MMON");
  if (root) roots.push(root);
  for (const user of users) {
    if (user === root) continue;
    if (!user.referredByCode || !byCode.has(user.referredByCode.toUpperCase())) roots.push(user);
  }
  const seen = new Set<string>();
  const rows: Array<{ user: User; depth: number }> = [];
  const walk = (user: User, depth: number) => {
    if (seen.has(user.id)) return;
    seen.add(user.id);
    rows.push({ user, depth });
    for (const child of children.get(user.referralCode.toUpperCase()) ?? []) walk(child, depth + 1);
  };
  roots.forEach((user) => walk(user, 0));
  users.forEach((user) => walk(user, 0));

  return (
    <details className={surface.referralDisclosure}>
      <summary><span><strong>Referral network</strong><small>Who invited whom</small></span><AppIcon name="chevron-down" size={16} /></summary>
      <div className={surface.tree}>
        {rows.map(({ user, depth }) => <div className={surface.treeNode} key={user.id} style={{ marginLeft: Math.min(depth, 8) * 18 }}><strong>{user.email} · {user.referralCode}</strong><span>{user.referredByCode ? `Invited by ${user.referredByCode}` : "Root / direct account"} · {user.inviteCount} direct · {status(user)}</span></div>)}
      </div>
    </details>
  );
}
