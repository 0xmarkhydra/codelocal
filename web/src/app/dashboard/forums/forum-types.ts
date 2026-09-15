import type { MessageKey } from "@/lib/i18n/messages";

export type ForumKind = "question" | "bug" | "idea";
export type ForumStatus = "open" | "under_review" | "planned" | "in_progress" | "resolved" | "closed";
export type ForumSeverity = "" | "low" | "medium" | "high" | "critical";

export type ForumTopic = {
  id: string;
  authorEmail: string;
  kind: ForumKind;
  title: string;
  body: string;
  status: ForumStatus;
  severity?: ForumSeverity;
  version?: string;
  environment?: string;
  reproductionSteps?: string;
  expectedBehavior?: string;
  actualBehavior?: string;
  tags: string[];
  assetIds: string[];
  githubIssueUrl?: string;
  githubIssueNumber?: number;
  githubPrUrl?: string;
  resolutionNote?: string;
  commentCount: number;
  voteCount: number;
  votedByViewer?: boolean;
  createdAt: number;
  updatedAt: number;
  resolvedAt?: number;
};

export type ForumComment = {
  id: string;
  topicId: string;
  authorEmail: string;
  body: string;
  assetIds: string[];
  createdAt: number;
  updatedAt: number;
};

export const kindLabels: Record<ForumKind, MessageKey> = {
  question: "Question / Problem",
  bug: "Bug Report",
  idea: "Idea / Feedback",
};

export const statusLabels: Record<ForumStatus, MessageKey> = {
  open: "Open",
  under_review: "Under review",
  planned: "Planned",
  in_progress: "In progress",
  resolved: "Resolved",
  closed: "Closed",
};

export const severityLabels: Record<Exclude<ForumSeverity, "">, MessageKey> = {
  low: "Low",
  medium: "Medium",
  high: "High",
  critical: "Critical",
};

export function isForumTopic(value: unknown): value is ForumTopic {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return typeof item.id === "string" && typeof item.title === "string" && typeof item.body === "string" &&
    (item.kind === "question" || item.kind === "bug" || item.kind === "idea") && typeof item.status === "string" &&
    typeof item.authorEmail === "string" && Array.isArray(item.tags) && Array.isArray(item.assetIds) && item.assetIds.every((assetID) => typeof assetID === "string") && typeof item.commentCount === "number" && typeof item.voteCount === "number";
}

export function formatForumTime(value: number, locale: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}
