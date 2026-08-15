package mcpgateway

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const (
	canonicalSemanticShadowTimeout  = 1200 * time.Millisecond
	canonicalSemanticHighSimilarity = .80
)

var canonicalSemanticShadowSlots = make(chan struct{}, 4)

func canonicalSemanticShadowEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_CANONICAL_EMBEDDING_SHADOW")))
	return value == "1" || value == "true" || value == "on"
}

func canonicalSemanticShadowSample(input cloud.CanonicalKnowledgeRecallInput, hits []cloud.CanonicalSemanticKnowledgeHit, duration time.Duration, recallErr error) cloud.CanonicalEmbeddingShadowSample {
	high := 0
	for _, hit := range hits {
		if hit.Similarity >= canonicalSemanticHighSimilarity {
			high++
		}
	}
	return cloud.CanonicalEmbeddingShadowSample{
		UserID: input.UserID, ProjectID: input.ProjectID, SemanticHits: len(hits), HighSimilarityHits: high,
		DurationMillis: duration.Milliseconds(), Errored: recallErr != nil,
	}
}

func (s *Service) maybeRunCanonicalSemanticShadow(input cloud.CanonicalKnowledgeRecallInput, query string) {
	if s == nil || s.Store == nil || knowledgeV2ReadMode() != knowledgeV2ReadModeShadow || !canonicalSemanticShadowEnabled() || strings.TrimSpace(query) == "" || strings.TrimSpace(input.ProjectID) == "" {
		return
	}
	select {
	case canonicalSemanticShadowSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-canonicalSemanticShadowSlots }()
		s.runCanonicalSemanticShadow(input, query)
	}()
}

func (s *Service) runCanonicalSemanticShadow(input cloud.CanonicalKnowledgeRecallInput, query string) {
	ctx, cancel := context.WithTimeout(context.Background(), canonicalSemanticShadowTimeout)
	started := time.Now()
	hits, recallErr := s.Store.RecallCanonicalKnowledgeSemantic(ctx, input, query)
	duration := time.Since(started)
	cancel()
	metricCtx, metricCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	metricErr := s.Store.RecordCanonicalEmbeddingShadowSample(metricCtx, canonicalSemanticShadowSample(input, hits, duration, recallErr))
	metricCancel()
	if metricErr != nil {
		slog.Debug("canonical semantic shadow metric skipped", "error", metricErr)
	}
	if recallErr != nil {
		slog.Debug("canonical semantic shadow recall skipped", "error", recallErr, "durationMs", duration.Milliseconds())
		return
	}
	high := 0
	for _, hit := range hits {
		if hit.Similarity >= canonicalSemanticHighSimilarity {
			high++
		}
	}
	slog.Debug("canonical semantic shadow recall", "semanticCount", len(hits), "highSimilarity", high, "durationMs", duration.Milliseconds())
}
