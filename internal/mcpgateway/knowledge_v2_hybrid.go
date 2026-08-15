package mcpgateway

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const (
	knowledgeV2HybridTimeout       = 220 * time.Millisecond
	knowledgeV2HybridMaxCanonical  = 2
	knowledgeV2HybridMinConfidence = .95
)

type canonicalHybridReader interface {
	canonicalKnowledgeReader
	KnowledgeV2Readiness(context.Context, string, string) (cloud.KnowledgeV2Readiness, error)
}

func hybridSummaryKey(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	return strings.TrimSpace(strings.TrimRight(value, ".;:"))
}

func canonicalHitMemoryRecord(hit cloud.CanonicalKnowledgeHit) (longmemory.Record, bool) {
	summary := longmemory.SanitizeText(hit.Revision.Summary, 1200)
	if summary == "" || hit.Knowledge.Confidence < knowledgeV2HybridMinConfidence {
		return longmemory.Record{}, false
	}
	scope := longmemory.ScopeProject
	if strings.TrimSpace(hit.Knowledge.RepositoryID) != "" {
		scope = longmemory.ScopeRepository
	}
	return longmemory.Record{
		ID:           "canonical:" + hit.Knowledge.KnowledgeID,
		UserID:       hit.Knowledge.UserID,
		ProjectID:    hit.Knowledge.ProjectID,
		RepositoryID: hit.Knowledge.RepositoryID,
		Scope:        scope,
		Level:        longmemory.LevelWorkspace,
		Kind:         hit.Knowledge.KnowledgeType,
		SourceType:   "knowledge_v2",
		Summary:      summary,
		Branch:       hit.Knowledge.Branch,
		Lifecycle:    longmemory.LifecycleActive,
		Confidence:   hit.Knowledge.Confidence,
		Importance:   hit.Knowledge.Importance,
		CreatedAt:    hit.Revision.CreatedAt,
		UpdatedAt:    hit.Knowledge.UpdatedAt,
	}, true
}

func mergeHybridCanonicalRecords(legacy []longmemory.Record, hits []cloud.CanonicalKnowledgeHit, limit int) []longmemory.Record {
	if limit <= 0 {
		limit = 6
	}
	if limit > 20 {
		limit = 20
	}
	seen := make(map[string]struct{}, len(legacy))
	for _, record := range legacy {
		if key := hybridSummaryKey(record.Summary); key != "" {
			seen[key] = struct{}{}
		}
	}
	canonical := make([]longmemory.Record, 0, knowledgeV2HybridMaxCanonical)
	for _, hit := range hits {
		record, ok := canonicalHitMemoryRecord(hit)
		if !ok {
			continue
		}
		key := hybridSummaryKey(record.Summary)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		canonical = append(canonical, record)
		if len(canonical) >= knowledgeV2HybridMaxCanonical || len(canonical) >= limit {
			break
		}
	}
	if len(canonical) == 0 {
		return append([]longmemory.Record(nil), legacy...)
	}
	legacyLimit := limit - len(canonical)
	if legacyLimit < 0 {
		legacyLimit = 0
	}
	if legacyLimit > len(legacy) {
		legacyLimit = len(legacy)
	}
	out := make([]longmemory.Record, 0, legacyLimit+len(canonical))
	out = append(out, legacy[:legacyLimit]...)
	out = append(out, canonical...)
	return out
}

func runGuardedHybridRecall(ctx context.Context, reader canonicalHybridReader, input cloud.CanonicalKnowledgeRecallInput, legacy []longmemory.Record) ([]longmemory.Record, bool, string) {
	if reader == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ProjectID) == "" {
		return append([]longmemory.Record(nil), legacy...), false, "hybrid_scope_unavailable"
	}
	readiness, err := reader.KnowledgeV2Readiness(ctx, input.UserID, input.ProjectID)
	if err != nil {
		return append([]longmemory.Record(nil), legacy...), false, "hybrid_readiness_error"
	}
	if readiness.Status != "ready" {
		return append([]longmemory.Record(nil), legacy...), false, "hybrid_not_ready"
	}
	hits, err := reader.RecallCanonicalKnowledge(ctx, input)
	if err != nil {
		return append([]longmemory.Record(nil), legacy...), false, "hybrid_recall_error"
	}
	merged := mergeHybridCanonicalRecords(legacy, hits, input.Limit)
	return merged, len(merged) != len(legacy) || !sameMemoryRecordIDs(merged, legacy), "hybrid_ready"
}

func sameMemoryRecordIDs(left, right []longmemory.Record) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID {
			return false
		}
	}
	return true
}

func (s *Service) maybeApplyHybridCanonicalRecall(parent context.Context, input cloud.CanonicalKnowledgeRecallInput, legacy []longmemory.Record) []longmemory.Record {
	if s == nil || s.Store == nil || knowledgeV2ReadMode() != knowledgeV2ReadModeHybrid || strings.TrimSpace(input.ProjectID) == "" {
		return legacy
	}
	ctx, cancel := context.WithTimeout(parent, knowledgeV2HybridTimeout)
	defer cancel()
	started := time.Now()
	merged, applied, reason := runGuardedHybridRecall(ctx, s.Store, input, legacy)
	slog.Debug("knowledge v2 guarded hybrid recall", "applied", applied, "reason", reason, "legacyCount", len(legacy), "resultCount", len(merged), "durationMs", time.Since(started).Milliseconds())
	return merged
}
