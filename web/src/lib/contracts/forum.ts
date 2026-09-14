export type PublicForumKind = "question" | "bug" | "idea";
export type PublicForumStatus = "open" | "under_review" | "planned" | "in_progress" | "resolved" | "closed";

export type PublicForumTopic = {
  id: string;
  author: string;
  kind: PublicForumKind;
  title: string;
  body: string;
  status: PublicForumStatus;
  severity?: string;
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
  createdAt: number;
  updatedAt: number;
  resolvedAt?: number;
};

export type PublicForumComment = {
  id: string;
  author: string;
  body: string;
  assetIds: string[];
  createdAt: number;
  updatedAt: number;
};

export type PublicForumCollection = { topics: PublicForumTopic[] };
export type PublicForumTopicResource = { topic: PublicForumTopic; comments: PublicForumComment[] };

function stringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

export function isPublicForumTopic(value: unknown): value is PublicForumTopic {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return typeof item.id === "string" && typeof item.author === "string" && typeof item.title === "string" && typeof item.body === "string" &&
    (item.kind === "question" || item.kind === "bug" || item.kind === "idea") &&
    (item.status === "open" || item.status === "under_review" || item.status === "planned" || item.status === "in_progress" || item.status === "resolved" || item.status === "closed") &&
    stringArray(item.tags) && stringArray(item.assetIds) && typeof item.commentCount === "number" && typeof item.voteCount === "number" &&
    typeof item.createdAt === "number" && typeof item.updatedAt === "number";
}

export function isPublicForumCollection(value: unknown): value is PublicForumCollection {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return Array.isArray(item.topics) && item.topics.every(isPublicForumTopic);
}

export function isPublicForumTopicResource(value: unknown): value is PublicForumTopicResource {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  if (!isPublicForumTopic(item.topic) || !Array.isArray(item.comments)) return false;
  return item.comments.every((comment) => {
    if (!comment || typeof comment !== "object") return false;
    const row = comment as Record<string, unknown>;
    return typeof row.id === "string" && typeof row.author === "string" && typeof row.body === "string" && stringArray(row.assetIds) &&
      typeof row.createdAt === "number" && typeof row.updatedAt === "number";
  });
}
