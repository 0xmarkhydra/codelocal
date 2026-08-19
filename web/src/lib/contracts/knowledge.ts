export type KnowledgeGraphNode = {
  id: string;
  kind: string;
  name: string;
  summary?: string;
  scope?: string;
  confidence: number;
  importance: number;
  lastSeenAt?: number;
};

export type KnowledgeGraphEdge = {
  id: string;
  from: string;
  to: string;
  relation: string;
  confidence: number;
  importance: number;
};

export type KnowledgeGraphResource = {
  meta: {
    nodeLimit: number;
    atNodeLimit: boolean;
  };
  stats: Record<string, number>;
  nodes: KnowledgeGraphNode[];
  edges: KnowledgeGraphEdge[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isUnitNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 1;
}

function isGraphNode(value: unknown): value is KnowledgeGraphNode {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" && value.id.length > 0 &&
    typeof value.kind === "string" &&
    typeof value.name === "string" &&
    (value.summary === undefined || typeof value.summary === "string") &&
    (value.scope === undefined || typeof value.scope === "string") &&
    isUnitNumber(value.confidence) &&
    isUnitNumber(value.importance) &&
    (value.lastSeenAt === undefined || (typeof value.lastSeenAt === "number" && Number.isFinite(value.lastSeenAt)))
  );
}

function isGraphEdge(value: unknown): value is KnowledgeGraphEdge {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" && value.id.length > 0 &&
    typeof value.from === "string" && value.from.length > 0 &&
    typeof value.to === "string" && value.to.length > 0 &&
    typeof value.relation === "string" &&
    isUnitNumber(value.confidence) &&
    isUnitNumber(value.importance)
  );
}

export function isKnowledgeGraphResource(value: unknown): value is KnowledgeGraphResource {
  if (!isRecord(value) || !isRecord(value.meta) || !isRecord(value.stats) || !Array.isArray(value.nodes) || !Array.isArray(value.edges)) {
    return false;
  }
  if (
    typeof value.meta.nodeLimit !== "number" || !Number.isInteger(value.meta.nodeLimit) || value.meta.nodeLimit <= 0 ||
    typeof value.meta.atNodeLimit !== "boolean" ||
    !Object.values(value.stats).every((item) => typeof item === "number" && Number.isFinite(item)) ||
    !value.nodes.every(isGraphNode) ||
    !value.edges.every(isGraphEdge)
  ) {
    return false;
  }
  const nodeIDs = new Set(value.nodes.map((node) => node.id));
  if (nodeIDs.size !== value.nodes.length) return false;
  const edgeIDs = new Set(value.edges.map((edge) => edge.id));
  if (edgeIDs.size !== value.edges.length) return false;
  return value.edges.every((edge) => edge.from !== edge.to && nodeIDs.has(edge.from) && nodeIDs.has(edge.to));
}
