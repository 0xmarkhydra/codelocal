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
	knowledgeV2ReadModeOff    = "off"
	knowledgeV2ReadModeShadow = "shadow"
	knowledgeV2ReadModeHybrid = "hybrid"
	knowledgeV2ShadowTimeout  = 450 * time.Millisecond
)

var knowledgeV2ShadowSlots = make(chan struct{}, 8)

type canonicalKnowledgeReader interface {
	RecallCanonicalKnowledge(context.Context, cloud.CanonicalKnowledgeRecallInput) ([]cloud.CanonicalKnowledgeHit, error)
}

type canonicalShadowMetrics struct {
	LegacyCount          int
	CanonicalCount       int
	RepositoryScoped     int
	BranchScoped         int
	HighConfidence       int
	DurationMilliseconds int64
}

func knowledgeV2ReadMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_KNOWLEDGE_V2_READ_MODE")))
	switch mode {
	case "", knowledgeV2ReadModeShadow:
		return knowledgeV2ReadModeShadow
	case knowledgeV2ReadModeOff, knowledgeV2ReadModeHybrid:
		return mode
	default:
		return knowledgeV2ReadModeOff
	}
}

func canonicalShadowMetricsForHits(legacyCount int, hits []cloud.CanonicalKnowledgeHit, duration time.Duration) canonicalShadowMetrics {
	metrics := canonicalShadowMetrics{LegacyCount: legacyCount, CanonicalCount: len(hits), DurationMilliseconds: duration.Milliseconds()}
	for _, hit := range hits {
		if strings.TrimSpace(hit.Knowledge.RepositoryID) != "" {
			metrics.RepositoryScoped++
		}
		if strings.TrimSpace(hit.Knowledge.Branch) != "" {
			metrics.BranchScoped++
		}
		if hit.Knowledge.Confidence >= .95 {
			metrics.HighConfidence++
		}
	}
	return metrics
}

func runCanonicalShadowRecall(ctx context.Context, reader canonicalKnowledgeReader, input cloud.CanonicalKnowledgeRecallInput, legacyCount int) (canonicalShadowMetrics, error) {
	started := time.Now()
	hits, err := reader.RecallCanonicalKnowledge(ctx, input)
	duration := time.Since(started)
	if err != nil {
		return canonicalShadowMetrics{LegacyCount: legacyCount, DurationMilliseconds: duration.Milliseconds()}, err
	}
	return canonicalShadowMetricsForHits(legacyCount, hits, duration), nil
}

func canonicalShadowSample(input cloud.CanonicalKnowledgeRecallInput, metrics canonicalShadowMetrics, recallErr error) cloud.KnowledgeV2ShadowSample {
	return cloud.KnowledgeV2ShadowSample{
		UserID: input.UserID, ProjectID: input.ProjectID,
		LegacyCount: metrics.LegacyCount, CanonicalCount: metrics.CanonicalCount, HighConfidenceHits: metrics.HighConfidence,
		DurationMillis: metrics.DurationMilliseconds, Errored: recallErr != nil,
	}
}

func (s *Service) maybeShadowCanonicalRecall(input cloud.CanonicalKnowledgeRecallInput, legacyCount int) {
	if s == nil || s.Store == nil || knowledgeV2ReadMode() != knowledgeV2ReadModeShadow || strings.TrimSpace(input.ProjectID) == "" {
		return
	}
	select {
	case knowledgeV2ShadowSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-knowledgeV2ShadowSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), knowledgeV2ShadowTimeout)
		defer cancel()
		metrics, recallErr := runCanonicalShadowRecall(ctx, s.Store, input, legacyCount)
		metricCtx, metricCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		metricErr := s.Store.RecordKnowledgeV2ShadowSample(metricCtx, canonicalShadowSample(input, metrics, recallErr))
		metricCancel()
		if metricErr != nil {
			slog.Debug("knowledge v2 shadow metric skipped", "error", metricErr)
		}
		if recallErr != nil {
			slog.Debug("knowledge v2 shadow recall skipped", "error", recallErr, "durationMs", metrics.DurationMilliseconds)
			return
		}
		slog.Debug("knowledge v2 shadow recall",
			"legacyCount", metrics.LegacyCount,
			"canonicalCount", metrics.CanonicalCount,
			"repositoryScoped", metrics.RepositoryScoped,
			"branchScoped", metrics.BranchScoped,
			"highConfidence", metrics.HighConfidence,
			"durationMs", metrics.DurationMilliseconds,
		)
	}()
}
