package mcpgateway

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const knowledgeV2SemanticHybridTimeout = cloud.CanonicalEmbeddingShadowSlowDuration + 50*time.Millisecond

var semanticCanaryMetricSlots = make(chan struct{}, 4)

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
		if hit.Similarity >= canonicalSemanticHighSimilarity {
			out = append(out, hit.CanonicalKnowledgeHit)
		}
	}
	return out
}

func semanticHybridTimedOut(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func runGuardedSemanticHybridRecall(ctx context.Context, reader canonicalSemanticHybridReader, input cloud.CanonicalKnowledgeRecallInput, query string, legacy []longmemory.Record) ([]longmemory.Record, bool, string) {
	if reader == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(query) == "" {
		return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonScopeUnavailable
	}
	readiness, err := reader.CanonicalSemanticReadiness(ctx, input.UserID, input.ProjectID)
	if err != nil {
		if semanticHybridTimedOut(ctx, err) {
			return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonTimeout
		}
		return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonReadinessError
	}
	if readiness.Status != "ready" {
		return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonNotReady
	}
	hits, err := reader.RecallCanonicalKnowledgeSemantic(ctx, input, query)
	if err != nil {
		if semanticHybridTimedOut(ctx, err) {
			return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonTimeout
		}
		return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonRecallError
	}
	eligible := semanticCanonicalHits(hits)
	if len(eligible) == 0 {
		return append([]longmemory.Record(nil), legacy...), false, cloud.SemanticCanaryReasonNoSimilarity
	}
	merged := mergeHybridCanonicalRecords(legacy, eligible, input.Limit)
	applied := len(merged) != len(legacy) || !sameMemoryRecordIDs(merged, legacy)
	if !applied {
		return merged, false, cloud.SemanticCanaryReasonNoUniqueClaims
	}
	return merged, true, cloud.SemanticCanaryReasonReady
}

func semanticHybridAppliedClaims(legacy, merged []longmemory.Record) int {
	seen := make(map[string]struct{}, len(legacy))
	for _, record := range legacy {
		seen[record.ID] = struct{}{}
	}
	count := 0
	for _, record := range merged {
		if _, existed := seen[record.ID]; !existed && strings.HasPrefix(record.ID, "canonical:") {
			count++
		}
	}
	if count > knowledgeV2HybridMaxCanonical {
		return knowledgeV2HybridMaxCanonical
	}
	return count
}

func (s *Service) recordSemanticCanaryAsync(input cloud.CanonicalKnowledgeRecallInput, reason string, duration time.Duration, appliedClaims int) {
	if s == nil || s.Store == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ProjectID) == "" {
		return
	}
	select {
	case semanticCanaryMetricSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-semanticCanaryMetricSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		err := s.Store.RecordCanonicalSemanticCanarySample(ctx, cloud.CanonicalSemanticCanarySample{
			UserID: input.UserID, ProjectID: input.ProjectID, Reason: reason,
			DurationMillis: duration.Milliseconds(), AppliedClaims: appliedClaims,
		})
		if err != nil {
			slog.Debug("semantic hybrid canary metric skipped", "error", err)
		}
	}()
}

func (s *Service) maybeApplySemanticHybridCanonicalRecall(parent context.Context, input cloud.CanonicalKnowledgeRecallInput, query string, legacy []longmemory.Record) ([]longmemory.Record, bool) {
	if s == nil || s.Store == nil || !canonicalSemanticHybridEnabled() || strings.TrimSpace(query) == "" {
		return legacy, false
	}
	gate := s.semanticCanaryGate(input)
	if !gate.Allow {
		slog.Debug("knowledge v2 semantic hybrid circuit breaker", "status", gate.Status)
		return legacy, false
	}
	ctx, cancel := context.WithTimeout(parent, knowledgeV2SemanticHybridTimeout)
	started := time.Now()
	merged, applied, reason := runGuardedSemanticHybridRecall(ctx, s.Store, input, query, legacy)
	duration := time.Since(started)
	cancel()
	claims := semanticHybridAppliedClaims(legacy, merged)
	s.observeSemanticCanaryOutcome(input, reason, duration, applied)
	s.recordSemanticCanaryAsync(input, reason, duration, claims)
	slog.Debug("knowledge v2 semantic hybrid canary", "applied", applied, "reason", reason, "legacyCount", len(legacy), "resultCount", len(merged), "durationMs", duration.Milliseconds())
	return merged, applied
}
