export type CodeGraphContext = {
  deviceId: string;
  deviceName: string;
  workspaceId: string;
  workspaceName: string;
  status: string;
  runtimeOnline: boolean;
};

export type CodeGraphRepository = {
  path: string;
  branch?: string;
  commit?: string;
  dirty: boolean;
  indexedAt: number;
  fileCount: number;
  symbolCount: number;
};

export type CodeGraphNode = {
  id: string;
  kind: string;
  name: string;
  summary?: string;
  path?: string;
  line?: number;
  column?: number;
  repositoryPath?: string;
  qualifiedName?: string;
  provider?: string;
  resolutionMode?: string;
  confidence: number;
  canonical: boolean;
  selected?: boolean;
};

export type CodeGraphEdge = {
  id: string;
  from: string;
  to: string;
  relation: string;
  provider?: string;
  resolutionMode?: string;
  confidence: number;
  count?: number;
};

export type CodeGraphImpact = {
  directCallers: number;
  directCallees: number;
  potentialCallers: number;
  affectedFiles: number;
  evidenceEdges: number;
  semanticEdges: number;
  averageConfidence: number;
  risk: string;
  truncated: boolean;
};

export type CodeGraphResource = {
  state: string;
  status: string;
  view: string;
  query: string;
  depth: number;
  maxNodes: number;
  truncated: boolean;
  context?: CodeGraphContext;
  repositories: CodeGraphRepository[];
  selectedId?: string;
  nodes: CodeGraphNode[];
  edges: CodeGraphEdge[];
  impact?: CodeGraphImpact;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0;
}

function isFiniteNonNegative(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function isUnit(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 1;
}

function isOptionalString(value: unknown) {
  return value === undefined || typeof value === "string";
}

function isOptionalCount(value: unknown) {
  return value === undefined || isCount(value);
}

function isContext(value: unknown): value is CodeGraphContext {
  if (!isRecord(value)) return false;
  return (
    typeof value.deviceId === "string" && typeof value.deviceName === "string" &&
    typeof value.workspaceId === "string" && typeof value.workspaceName === "string" &&
    typeof value.status === "string" && typeof value.runtimeOnline === "boolean"
  );
}

function isRepository(value: unknown): value is CodeGraphRepository {
  if (!isRecord(value)) return false;
  return (
    typeof value.path === "string" && isOptionalString(value.branch) && isOptionalString(value.commit) &&
    typeof value.dirty === "boolean" && isFiniteNonNegative(value.indexedAt) && isCount(value.fileCount) && isCount(value.symbolCount)
  );
}

function isNode(value: unknown): value is CodeGraphNode {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" && /^n[1-9][0-9]*$/.test(value.id) && typeof value.kind === "string" && typeof value.name === "string" &&
    isOptionalString(value.summary) && isOptionalString(value.path) && isOptionalCount(value.line) && isOptionalCount(value.column) &&
    isOptionalString(value.repositoryPath) && isOptionalString(value.qualifiedName) && isOptionalString(value.provider) &&
    isOptionalString(value.resolutionMode) && isUnit(value.confidence) && typeof value.canonical === "boolean" &&
    (value.selected === undefined || typeof value.selected === "boolean")
  );
}

function isEdge(value: unknown): value is CodeGraphEdge {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" && /^e[1-9][0-9]*$/.test(value.id) && /^n[1-9][0-9]*$/.test(String(value.from)) &&
    /^n[1-9][0-9]*$/.test(String(value.to)) && typeof value.relation === "string" && isOptionalString(value.provider) &&
    isOptionalString(value.resolutionMode) && isUnit(value.confidence) && isOptionalCount(value.count)
  );
}

function isImpact(value: unknown): value is CodeGraphImpact {
  if (!isRecord(value)) return false;
  return (
    isCount(value.directCallers) && isCount(value.directCallees) && isCount(value.potentialCallers) && isCount(value.affectedFiles) &&
    isCount(value.evidenceEdges) && isCount(value.semanticEdges) && isUnit(value.averageConfidence) &&
    typeof value.risk === "string" && typeof value.truncated === "boolean"
  );
}

export function isCodeGraphResource(value: unknown): value is CodeGraphResource {
  if (!isRecord(value) || !Array.isArray(value.repositories) || !Array.isArray(value.nodes) || !Array.isArray(value.edges)) return false;
  if (
    typeof value.state !== "string" || typeof value.status !== "string" || typeof value.view !== "string" || typeof value.query !== "string" ||
    !isCount(value.depth) || !isCount(value.maxNodes) || typeof value.truncated !== "boolean" ||
    (value.context !== undefined && !isContext(value.context)) || (value.selectedId !== undefined && typeof value.selectedId !== "string") ||
    (value.impact !== undefined && !isImpact(value.impact))
  ) return false;
  if (!value.repositories.every(isRepository) || !value.nodes.every(isNode) || !value.edges.every(isEdge)) return false;
  const ids = new Set(value.nodes.map((node) => node.id));
  if (value.selectedId && !ids.has(value.selectedId)) return false;
  return value.edges.every((edge) => ids.has(edge.from) && ids.has(edge.to) && edge.from !== edge.to);
}
