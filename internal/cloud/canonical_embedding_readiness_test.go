package cloud

import (
	"strings"
	"testing"
	"time"
)

func semanticReadyBase() KnowledgeV2Readiness {
	return KnowledgeV2Readiness{Status: "ready"}
}

func semanticCurrentEmbedding() CanonicalEmbeddingFreshness {
	return CanonicalEmbeddingFreshness{Status: "current", Dimensions: 768}
}

func semanticReadyMetrics(now time.Time) *CanonicalEmbeddingShadowMetrics {
	return &CanonicalEmbeddingShadowMetrics{
		SamplesTotal: 100, SuccessCount: 99, ErrorCount: 1, SemanticNonemptySamples: 90,
		SemanticHitsTotal: 100, HighSimilarityHitsTotal: 80, SlowSamples: 5, LastSampleAt: now.UnixMilli(),
	}
}

func TestCanonicalSemanticReadinessRequiresBaseAndCurrentEmbedding(t *testing.T) {
	now := time.Now()
	thresholds := canonicalSemanticReadinessThresholds{MinSamples: 10, MaxErrorPercent: 5, MaxSlowPercent: 20, MinSemanticHits: 10, MinHighSimilarityPercent: 60, Freshness: 24 * time.Hour}
	blockedBase := canonicalSemanticReadinessFrom("u", "p", semanticReadyMetrics(now), KnowledgeV2Readiness{Status: "blocked"}, semanticCurrentEmbedding(), thresholds, now)
	if blockedBase.Status != "blocked" || len(blockedBase.ReasonCodes) != 1 || blockedBase.ReasonCodes[0] != "knowledge_v2_not_ready" {
		t.Fatalf("semantic readiness ignored blocked base V2: %#v", blockedBase)
	}
	staleEmbedding := canonicalSemanticReadinessFrom("u", "p", semanticReadyMetrics(now), semanticReadyBase(), CanonicalEmbeddingFreshness{Status: "stale"}, thresholds, now)
	if staleEmbedding.Status != "collecting" || staleEmbedding.ReasonCodes[0] != "embedding_index_not_current" {
		t.Fatalf("semantic readiness ignored stale embedding index: %#v", staleEmbedding)
	}
}

func TestCanonicalSemanticReadinessCollectsBlocksAndPassesDeterministically(t *testing.T) {
	now := time.Now()
	thresholds := canonicalSemanticReadinessThresholds{MinSamples: 50, MaxErrorPercent: 2, MaxSlowPercent: 10, MinSemanticHits: 20, MinHighSimilarityPercent: 60, Freshness: 24 * time.Hour}
	collectingMetrics := semanticReadyMetrics(now)
	collectingMetrics.SamplesTotal = 10
	collecting := canonicalSemanticReadinessFrom("u", "p", collectingMetrics, semanticReadyBase(), semanticCurrentEmbedding(), thresholds, now)
	if collecting.Status != "collecting" || collecting.ReasonCodes[0] != "insufficient_semantic_shadow_samples" {
		t.Fatalf("unexpected semantic collecting state: %#v", collecting)
	}
	blockedMetrics := semanticReadyMetrics(now)
	blockedMetrics.ErrorCount, blockedMetrics.SlowSamples, blockedMetrics.HighSimilarityHitsTotal = 10, 20, 30
	blocked := canonicalSemanticReadinessFrom("u", "p", blockedMetrics, semanticReadyBase(), semanticCurrentEmbedding(), thresholds, now)
	if blocked.Status != "blocked" || strings.Join(blocked.ReasonCodes, ",") != "semantic_shadow_error_rate_high,semantic_shadow_slow_rate_high,semantic_similarity_ratio_low" {
		t.Fatalf("unexpected semantic blocked state: %#v", blocked)
	}
	ready := canonicalSemanticReadinessFrom("u", "p", semanticReadyMetrics(now), semanticReadyBase(), semanticCurrentEmbedding(), thresholds, now)
	if ready.Status != "ready" || ready.ReasonCodes[0] != "semantic_shadow_gate_passed" || ready.HighSimilarityPercent != 80 {
		t.Fatalf("unexpected semantic ready state: %#v", ready)
	}
}

func TestCanonicalSemanticReadinessThresholdsAreClamped(t *testing.T) {
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_READY_MIN_SAMPLES", "1")
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_READY_MAX_ERROR_PERCENT", "99")
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_READY_MAX_SLOW_PERCENT", "99")
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_READY_MIN_HITS", "1")
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_READY_MIN_HIGH_SIMILARITY_PERCENT", "1")
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_READY_FRESH_DAYS", "99")
	got := canonicalSemanticReadinessThresholdConfig()
	if got.MinSamples != 10 || got.MaxErrorPercent != 25 || got.MaxSlowPercent != 50 || got.MinSemanticHits != 1 || got.MinHighSimilarityPercent != 40 || got.Freshness != 30*24*time.Hour {
		t.Fatalf("semantic thresholds not clamped: %#v", got)
	}
}

func TestCanonicalSemanticReadinessMetricQueryIsTenantProjectScopedAndAggregateOnly(t *testing.T) {
	lower := strings.ToLower(canonicalEmbeddingShadowMetricsSelectSQL)
	if !strings.Contains(lower, "where user_id=$1 and project_id=$2") {
		t.Fatalf("semantic readiness metric query lost tenant/project scope: %s", lower)
	}
	for _, forbidden := range []string{"summary", "stable_key", "query_text", "repository_id", "branch", "objective", "root_cause"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("semantic readiness metric query exposes content field %q", forbidden)
		}
	}
}
