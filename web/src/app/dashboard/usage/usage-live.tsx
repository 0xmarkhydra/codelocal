"use client";

import { isUsageResource } from "@/lib/contracts/usage";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { DashboardIcon } from "../dashboard-icon";
import visual from "../visual-dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

const compactNumber = new Intl.NumberFormat("en-US", { notation:"compact", maximumFractionDigits:1 });
const exactNumber = new Intl.NumberFormat("en-US");

export function LiveUsage() {
  const { state, retry } = useDashboardResource("/api/v1/usage", isUsageResource);
  if (state.kind !== "ready") return <DashboardResourceFeedback label="Usage" {...(state.kind === "error" ? {kind:"error" as const,message:state.message,onRetry:retry}:{kind:state.kind})}/>;
  const resource = state.value;
  const max = Math.max(resource.last24h.totalTokensEstimated,resource.last30d.totalTokensEstimated,resource.allTime.totalTokensEstimated,1);
  const bars = [resource.last24h,resource.last30d,resource.allTime];
  return <div className={visual.shell} aria-live="polite">
    <div className={visual.usageTop}>
      <article className={visual.usageCard}><span className={visual.statIcon}><DashboardIcon name="tokens"/></span><div><span>24h</span><strong>{compactNumber.format(resource.last24h.totalTokensEstimated)}</strong></div></article>
      <article className={visual.usageCard}><span className={visual.statIcon}><DashboardIcon name="calls"/></span><div><span>30d</span><strong>{compactNumber.format(resource.last30d.totalTokensEstimated)}</strong></div></article>
      <article className={visual.usageCard}><span className={visual.statIcon}><DashboardIcon name="activity"/></span><div><span>All time</span><strong>{compactNumber.format(resource.allTime.totalTokensEstimated)}</strong></div></article>
    </div>
    <section className={`${visual.panel} ${visual.chart}`} aria-hidden="true"><div className={visual.chartGrid}/><svg viewBox="0 0 1000 260" preserveAspectRatio="none"><path className={visual.chartPathA} d="M0 190 C120 80 210 180 320 120 S520 40 620 130 S820 190 1000 85"/><path className={visual.chartPathB} d="M0 215 C130 155 220 205 340 170 S560 105 670 155 S850 100 1000 145"/></svg></section>
    <section className={visual.panel}><div className={visual.breakdown}>{bars.map((value,index)=><div className={visual.breakRow} key={index}><span className={visual.rowIcon}><DashboardIcon name={index===0?"bolt":index===1?"activity":"layers"} size={14}/></span><span>{index===0?"24h":index===1?"30d":"All"}</span><div className={visual.progress}><i style={{width:`${Math.max(7,Math.round((value.totalTokensEstimated/max)*100))}%`}}/></div><strong>{exactNumber.format(value.calls)}</strong></div>)}</div></section>
  </div>;
}
