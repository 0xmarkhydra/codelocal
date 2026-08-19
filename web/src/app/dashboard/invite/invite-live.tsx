"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { CopyButton } from "../copy-button";
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
  if (status === "mcp_active") return "Using MCP now";
  if (status === "runtime_online") return "Runtime online";
  return "Offline";
}

export function InviteLive() {
  const router = useRouter();
  const [data, setData] = useState<Invite | null>(null);
  const [error, setError] = useState(false);
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

  if (error) return <section className={dashboard.livePanel}><span className={dashboard.eyebrow}>Invite</span><h2>Invite data is temporarily unavailable.</h2><p>Refresh after the Go backend is reachable again.</p></section>;
  if (!data) return <section className={dashboard.livePanel}><span className={dashboard.eyebrow}>Invite</span><h2>Loading your invite network…</h2></section>;

  return <div className={surface.grid}>
    <section className={surface.card}>
      <span className={dashboard.eyebrow}>Your invite</span>
      <h2 className={surface.title}>Invite code</h2>
      <p className={surface.copy}>Share this code with people you trust. Invited by: {data.invitedBy}.</p>
      <div className={surface.codeRow}><div className={surface.code}>{data.referralCode}</div><CopyButton value={data.referralCode} label="Copy code" /></div>
    </section>
    <div className={surface.metrics}>
      <div className={surface.metric}><span>Direct invites</span><strong>{data.directCount}</strong></div>
      <div className={surface.metric}><span>Active invites</span><strong>{data.activeCount}</strong></div>
      <div className={surface.metric}><span>Invite link</span><strong>Ready</strong></div>
    </div>
    <section className={surface.card}>
      <span className={dashboard.eyebrow}>Share link</span>
      <div className={surface.codeRow}><div className={surface.code}>{data.inviteLink}</div><CopyButton value={data.inviteLink} label="Copy link" /></div>
    </section>
    <section className={surface.card}>
      <span className={dashboard.eyebrow}>People you invited</span>
      <h2 className={surface.title}>{data.directCount} direct referral{data.directCount === 1 ? "" : "s"}</h2>
      {data.members.length === 0 ? <div className={surface.empty}>No one has joined with your invite code yet.</div> : <div className={surface.list}>{data.members.map((member, index) => <div className={surface.row} key={`${member.emailMasked}-${member.joinedAt}-${index}`}><div className={surface.identity}><strong>{member.initial} · {member.emailMasked}</strong><span>Joined {new Date(member.joinedAt).toLocaleString()}</span></div><span className={`${surface.badge} ${member.status === "mcp_active" ? surface.badgeGreen : member.status === "runtime_online" ? surface.badgeBlue : ""}`}>{statusLabel(member.status)}</span></div>)}</div>}
    </section>
  </div>;
}
