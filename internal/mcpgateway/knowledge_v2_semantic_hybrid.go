package mcpgateway

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const knowledgeV2SemanticHybridTimeout = cloud.CanonicalEmbeddingShadowSlowDuration + 50*time.Millisecond

type canonicalSemanticHybridReader interface {
	canonicalHybridReader
	CanonicalSemanticReadiness(context.Context, string, string) (cloud.CanonicalSemanticReadiness, error)
	RecallCanonicalKnowledgeSemantic(context.Context, cloud.CanonicalKnowledgeRecallInput, string) ([]cloud.CanonicalSemanticKnowledgeHit, error)
}

func canonicalSemanticHybridEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_CANONICAL_SEMANTIC_HYBRID")))
	return value == "1" || value == "true" || value == "on"
}

func semanticCanonicalHits(hits []cloud.CanonicalSemanticKnowledgeHit) []cloud.CanonicalKnowledgeHit {
	out := make([]cloud.CanonicalKnowledgeHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Similarity < canonicalSemanticHighSimilarity {
			continue
		}
		out = append(out, hit.CanonicalKnowledgeHit)
	}
	return out
}

func runGuardedSemanticHybridRecall(ctx context.Context, reader canonicalSemanticHybridReader, input cloud.CanonicalKnowledgeRecallInput, query string, legacy []longmemory.Record) ([]longmemory.Record, bool, string) {
	if reader == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(query) == "" {
		return append([]longmemory.Record(nil), legacy...), false, "semantic_hybrid_scope_unavailable"
	}
	readiness, err := reader.CanonicalSemanticReadiness(ctx, input.UserID, input.ProjectID)
	if err != nil {
		return append([]longmemory.Record(nil), legacy...), false, "semantic_hybrid_readiness_error"
	}
	if readiness.Status != "ready" {
		return append([]longmemory.Record(nil), legacy...), false, "semantic_hybrid_not_ready"
	}
	hits, err := reader.RecallCanonicalKnowledgeSemantic(ctx, input, query)
	if err != nil {
		return append([]longmemory.Record(nil), legacy...), false, "semantic_hybrid_recall_error"
	}
	eligible := semanticCanonicalHits(hits)
	if len(eligible) == 0 {
		return append([]longmemory.Record(nil), legacy...), false, "semantic_hybrid_no_high_similarity_hits"
	}
	merged := mergeHybridCanonicalRecords(legacy, eligible, input.Limit)
	return merged, len(merged) != len(legacy) || !sameMemoryRecordIDs(merged, legacy), "semantic_hybrid_ready"
}

func (s *Service) maybeApplySemanticHybridCanonicalRecall(parent context.Context, input cloud.CanonicalKnowledgeRecallInput, query string, legacy []longmemory.Record) ([]longmemory.Record, bool) {
	if s == nil || s.Store == nil || !canonicalSemanticHybridEnabled() || strings.TrimSpace(query) == "" {
		return legacy, false
	}
	ctx, cancel := context.WithTimeout(parent, knowledgeV2SemanticHybridTimeout)
	started := time.Now()
	merged, applied, reason := runGuardedSemanticHybridRecall(ctx, s.Store, input, query, legacy)
	cancel()
	slog.Debug("knowledge v2 semantic hybrid canary", "applied", applied, "reason", reason, "legacyCount", len(legacy), "resultCount", len(merged), "durationMs", time.Since(started).Milliseconds())
	return merged, applied
}
