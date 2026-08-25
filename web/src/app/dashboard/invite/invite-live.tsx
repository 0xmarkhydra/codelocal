"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { CopyButton } from "../copy-button";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import dashboard from "../dashboard.module.css";
import surface from "../dashboard-surfaces.module.css";

type Member = { emailMasked: string; initial: string; joinedAt: number; status: string };
type Invite = { referralCode: string; invitedBy: string; inviteLink: string; directCount: number; activeCount: number; members: Member[] };

function isInvite(value: unknown): value is Invite {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return typeof v.referralCode === "string" && typeof v.invitedBy === "string" && typeof v.inviteLink === "string" && typeof v.directCount === "number" && typeof v.activeCount === "number" && Array.isArray(v.members);
}

function statusLabel(status: string) {
  if (status === "mcp_active") return "Đang dùng";
  if (status === "runtime_online") return "Online";
  return "Offline";
}

export function InviteLive() {
  const router = useRouter();
  const [data, setData] = useState<Invite | null>(null);
  const [error, setError] = useState(false);
  const [filter, setFilter] = useState<"all" | "active">("all");

  useEffect(() => {
    fetch("/api/v1/invite", { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (response.status === 401) { router.replace(`/login?next=${encodeURIComponent("/dashboard/invite")}`); throw new Error("unauthorized"); }
        if (!response.ok) throw new Error("unavailable");
        return response.json() as Promise<unknown>;
      })
      .then((payload) => { if (!isInvite(payload)) throw new Error("invalid"); setData(payload); })
      .catch((reason) => { if (reason instanceof Error && reason.message !== "unauthorized") setError(true); });
  }, [router]);

  const members = useMemo(() => {
    if (!data) return [];
    if (filter === "active") return data.members.filter((member) => member.status === "mcp_active" || member.status === "runtime_online");
    return data.members;
  }, [data, filter]);

  if (error) return <DashboardResourceFeedback kind="error" label="Invite" message="Invite tạm thời không khả dụng" onRetry={() => window.location.reload()} />;
  if (!data) return <DashboardResourceFeedback kind="loading" label="Invite" />;

  return (
    <div className={surface.inviteLayout}>
      <section className={surface.inviteHero}>
        <div className={surface.inviteHeroCopy}>
          <span className={dashboard.eyebrow}>Invite link</span>
          <h2>Mời mọi người vào CodeLocal</h2>
          <p>Chia sẻ link hoặc mã mời. Trạng thái thành viên sẽ cập nhật từ backend hiện có.</p>
        </div>
        <div className={surface.inviteLinkBox}>
          <div>
            <small>Link của bạn</small>
            <strong title={data.inviteLink}>{data.inviteLink}</strong>
          </div>
          <CopyButton value={data.inviteLink} label="Copy link" />
        </div>
        <div className={surface.inviteCodeRow}>
          <span>Mã mời <strong>{data.referralCode}</strong></span>
          <CopyButton value={data.referralCode} label="Copy code" />
        </div>
      </section>

      <section className={surface.memberPanel}>
        <div className={surface.memberPanelHead}>
          <div>
            <span className={dashboard.eyebrow}>Members</span>
            <h2>{data.directCount} người đã tham gia</h2>
          </div>
          <div className={surface.memberFilters} role="tablist" aria-label="Invite filters">
            <button type="button" data-active={filter === "all" || undefined} onClick={() => setFilter("all")}>Tất cả {data.directCount}</button>
            <button type="button" data-active={filter === "active" || undefined} onClick={() => setFilter("active")}>Đang hoạt động {data.activeCount}</button>
          </div>
        </div>

        {members.length === 0 ? <div className={surface.empty}>Chưa có thành viên phù hợp.</div> : (
          <div className={surface.memberList}>
            {members.map((member, index) => (
              <div className={surface.memberRow} key={`${member.emailMasked}-${member.joinedAt}-${index}`}>
                <span className={surface.memberAvatar} aria-hidden="true">{member.initial}</span>
                <div className={surface.identity}><strong>{member.emailMasked}</strong><span>Tham gia {new Date(member.joinedAt).toLocaleString()}</span></div>
                <span className={surface.memberStatus} data-state={member.status}><i />{statusLabel(member.status)}</span>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
