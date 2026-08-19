export type KnowledgePipelineHealth = {
  available: boolean;
  status: string;
  pendingCount: number;
  processingCount: number;
  retryingCount: number;
  deadCount: number;
  processedLastHour: number;
  oldestActiveAgeMs: number;
};

export type KnowledgeIndexHealth = {
  available: boolean;
  status: string;
  projectCount: number;
  currentProjects: number;
  emptyProjects: number;
  staleProjects: number;
  missingProjects: number;
  maxLagMs: number;
};

export type KnowledgeCanary = {
  available: boolean;
  attemptsTotal: number;
  appliedCount: number;
  appliedClaimsTotal: number;
  deterministicFallbackCount: number;
  readinessBlockedCount: number;
  errorCount: number;
  timeoutCount: number;
  slowCount: number;
};

export type CollectiveRecommendation = {
  taskKind: string;
  checkProfile: string[];
  fileCountBucket: string;
  symbolCountBucket: string;
  executionTool: string;
  qualityBucket: string;
  skillUsed: boolean;
  diffObserved: boolean;
  contributorCount: number;
  sampleCount: number;
  meanUserSuccessRate: number;
  lastSeenAt: number;
};

export type KnowledgeCollective = {
  available: boolean;
  contributionEnabled: boolean;
  suggestionsEnabled: boolean;
  contributionAvailable: boolean;
  suggestionsAvailable: boolean;
  minimumContributors: number;
  recommendations: CollectiveRecommendation[];
};

export type KnowledgeHealthResource = {
  csrf: string;
  pipeline: KnowledgePipelineHealth;
  graphIndex: KnowledgeIndexHealth;
  semanticIndex: KnowledgeIndexHealth;
  canary: KnowledgeCanary;
  collective: KnowledgeCollective;
  privacy: {
    rawCodeShared: boolean;
    conversationShared: boolean;
    projectIdentityShared: boolean;
    localReplayTrustShared: boolean;
  };
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

function isPipeline(value: unknown): value is KnowledgePipelineHealth {
  if (!isRecord(value)) return false;
  return (
    typeof value.available === "boolean" && typeof value.status === "string" &&
    isCount(value.pendingCount) && isCount(value.processingCount) && isCount(value.retryingCount) &&
    isCount(value.deadCount) && isCount(value.processedLastHour) && isFiniteNonNegative(value.oldestActiveAgeMs)
  );
}

function isIndex(value: unknown): value is KnowledgeIndexHealth {
  if (!isRecord(value)) return false;
  return (
    typeof value.available === "boolean" && typeof value.status === "string" &&
    isCount(value.projectCount) && isCount(value.currentProjects) && isCount(value.emptyProjects) &&
    isCount(value.staleProjects) && isCount(value.missingProjects) && isFiniteNonNegative(value.maxLagMs)
  );
}

function isCanary(value: unknown): value is KnowledgeCanary {
  if (!isRecord(value)) return false;
  return (
    typeof value.available === "boolean" && isCount(value.attemptsTotal) && isCount(value.appliedCount) &&
    isCount(value.appliedClaimsTotal) && isCount(value.deterministicFallbackCount) && isCount(value.readinessBlockedCount) &&
    isCount(value.errorCount) && isCount(value.timeoutCount) && isCount(value.slowCount)
  );
}

function isRecommendation(value: unknown): value is CollectiveRecommendation {
  if (!isRecord(value)) return false;
  return (
    typeof value.taskKind === "string" && Array.isArray(value.checkProfile) && value.checkProfile.every((item) => typeof item === "string") &&
    typeof value.fileCountBucket === "string" && typeof value.symbolCountBucket === "string" && typeof value.executionTool === "string" &&
    typeof value.qualityBucket === "string" && typeof value.skillUsed === "boolean" && typeof value.diffObserved === "boolean" &&
    isCount(value.contributorCount) && isCount(value.sampleCount) && isUnit(value.meanUserSuccessRate) && isFiniteNonNegative(value.lastSeenAt)
  );
}

function isCollective(value: unknown): value is KnowledgeCollective {
  if (!isRecord(value) || !Array.isArray(value.recommendations)) return false;
  return (
    typeof value.available === "boolean" && typeof value.contributionEnabled === "boolean" && typeof value.suggestionsEnabled === "boolean" &&
    typeof value.contributionAvailable === "boolean" && typeof value.suggestionsAvailable === "boolean" &&
    isCount(value.minimumContributors) && value.recommendations.every(isRecommendation)
  );
}

export function isKnowledgeHealthResource(value: unknown): value is KnowledgeHealthResource {
  if (!isRecord(value) || !isRecord(value.privacy)) return false;
  return (
    typeof value.csrf === "string" && /^[A-Za-z0-9_-]{24,128}$/.test(value.csrf) &&
    isPipeline(value.pipeline) && isIndex(value.graphIndex) && isIndex(value.semanticIndex) && isCanary(value.canary) &&
    isCollective(value.collective) &&
    typeof value.privacy.rawCodeShared === "boolean" && typeof value.privacy.conversationShared === "boolean" &&
    typeof value.privacy.projectIdentityShared === "boolean" && typeof value.privacy.localReplayTrustShared === "boolean"
  );
}
