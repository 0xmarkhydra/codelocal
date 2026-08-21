"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";

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
function status(user: User) { return user.mcpActive ? "Using MCP now" : user.runtimeActive ? "Runtime online" : "Offline"; }
function when(value: number) { return value > 0 ? new Date(value).toLocaleString() : "Never"; }

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

  if (error === "forbidden") return <section className={dashboard.livePanel}><h2>Admin only</h2></section>;
  if (error) return <section className={dashboard.livePanel}><h2>Unavailable</h2></section>;
  if (!data) return <section className={dashboard.livePanel}><h2>Loading…</h2></section>;

  return <div className={surface.grid}>
    <div className={surface.metrics}>
      <div className={surface.metric}><span>Total users</span><strong>{data.totalUsers}</strong></div>
      <div className={surface.metric}><span>Active users</span><strong>{data.activeUsers}</strong></div>
      <div className={surface.metric}><span>Runtime online / MCP now</span><strong>{data.runtimeOnline} / {data.usingMcpNow}</strong></div>
    </div>
    <section className={surface.card}>
      <h2 className={surface.title}>Users</h2>
      <input className={surface.search} type="search" value={query} onChange={(event) => { setQuery(event.target.value); setPage(1); }} placeholder="Search email or referral code" />
      <div className={surface.adminTable}>
        {visible.map((user) => <div className={surface.adminRow} key={user.id}>
          <div className={surface.identity}><strong>{user.email}</strong><span>Joined {when(user.createdAt)}</span></div>
          <div><strong>{status(user)}</strong><span>{user.runtimeActive ? "runtime live" : "runtime offline"}</span></div>
          <div><strong>{user.referralCode}</strong><span>Invited by {user.referredByCode || "—"} · {user.inviteCount} direct</span></div>
          <div><strong>MCP {when(user.lastMcpUsedAt)}</strong><span>Device {when(user.lastDeviceSeenAt)}</span></div>
        </div>)}
      </div>
      <div className={dashboard.previewActions}>
        <button className={dashboard.liveAction} type="button" disabled={safePage <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>Previous</button>
        <span className={surface.badge}>Page {safePage} of {totalPages}</span>
        <button className={dashboard.liveAction} type="button" disabled={safePage >= totalPages} onClick={() => setPage((p) => Math.min(totalPages, p + 1))}>Next</button>
      </div>
    </section>
    <ReferralTree users={data.users} />
  </div>;
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

  return <section className={surface.card}>
    <span className={dashboard.eyebrow}>Referral network</span>
    <h2 className={surface.title}>Who invited whom</h2>
    <div className={surface.tree}>{rows.map(({ user, depth }) => <div className={surface.treeNode} key={user.id} style={{ marginLeft: Math.min(depth, 8) * 18 }}><strong>{user.email} · {user.referralCode}</strong><span>{user.referredByCode ? `Invited by ${user.referredByCode}` : "Root / direct account"} · {user.inviteCount} direct · {status(user)}</span></div>)}</div>
  </section>;
}
