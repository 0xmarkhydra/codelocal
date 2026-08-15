package cloud

import (
	"context"
	"errors"
	"strings"
	"time"
)

const canonicalEmbeddingShadowMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_embedding_shadow_metrics (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 samples_total BIGINT NOT NULL DEFAULT 0,
 success_count BIGINT NOT NULL DEFAULT 0,
 error_count BIGINT NOT NULL DEFAULT 0,
 semantic_nonempty_samples BIGINT NOT NULL DEFAULT 0,
 semantic_hits_total BIGINT NOT NULL DEFAULT 0,
 high_similarity_hits_total BIGINT NOT NULL DEFAULT 0,
 slow_samples BIGINT NOT NULL DEFAULT 0,
 duration_millis_total BIGINT NOT NULL DEFAULT 0,
 last_duration_millis BIGINT NOT NULL DEFAULT 0,
 max_duration_millis BIGINT NOT NULL DEFAULT 0,
 first_sample_at BIGINT NOT NULL,
 last_sample_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_embedding_shadow_metrics_last
 ON codelocal_knowledge_embedding_shadow_metrics(last_sample_at DESC);
`

const upsertCanonicalEmbeddingShadowMetricsSQL = `
INSERT INTO codelocal_knowledge_embedding_shadow_metrics(
 user_id,project_id,samples_total,success_count,error_count,semantic_nonempty_samples,semantic_hits_total,
 high_similarity_hits_total,slow_samples,duration_millis_total,last_duration_millis,max_duration_millis,first_sample_at,last_sample_at)
VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$9,$9,$10,$10)
ON CONFLICT(user_id,project_id) DO UPDATE SET
 samples_total=codelocal_knowledge_embedding_shadow_metrics.samples_total+1,
 success_count=codelocal_knowledge_embedding_shadow_metrics.success_count+EXCLUDED.success_count,
 error_count=codelocal_knowledge_embedding_shadow_metrics.error_count+EXCLUDED.error_count,
 semantic_nonempty_samples=codelocal_knowledge_embedding_shadow_metrics.semantic_nonempty_samples+EXCLUDED.semantic_nonempty_samples,
 semantic_hits_total=codelocal_knowledge_embedding_shadow_metrics.semantic_hits_total+EXCLUDED.semantic_hits_total,
 high_similarity_hits_total=codelocal_knowledge_embedding_shadow_metrics.high_similarity_hits_total+EXCLUDED.high_similarity_hits_total,
 slow_samples=codelocal_knowledge_embedding_shadow_metrics.slow_samples+EXCLUDED.slow_samples,
 duration_millis_total=codelocal_knowledge_embedding_shadow_metrics.duration_millis_total+EXCLUDED.duration_millis_total,
 last_duration_millis=EXCLUDED.last_duration_millis,
 max_duration_millis=GREATEST(codelocal_knowledge_embedding_shadow_metrics.max_duration_millis,EXCLUDED.max_duration_millis),
 last_sample_at=EXCLUDED.last_sample_at`

type CanonicalEmbeddingShadowSample struct {
	UserID             string
	ProjectID          string
	SemanticHits       int
	HighSimilarityHits int
	DurationMillis     int64
	Errored            bool
}

type CanonicalEmbeddingShadowMetrics struct {
	SamplesTotal            int64 `json:"samplesTotal"`
	SuccessCount            int64 `json:"successCount"`
	ErrorCount              int64 `json:"errorCount"`
	SemanticNonemptySamples int64 `json:"semanticNonemptySamples"`
	SemanticHitsTotal       int64 `json:"semanticHitsTotal"`
	HighSimilarityHitsTotal int64 `json:"highSimilarityHitsTotal"`
	SlowSamples             int64 `json:"slowSamples"`
	DurationMillisTotal     int64 `json:"durationMillisTotal"`
	LastDurationMillis      int64 `json:"lastDurationMillis"`
	MaxDurationMillis       int64 `json:"maxDurationMillis"`
	FirstSampleAt           int64 `json:"firstSampleAt"`
	LastSampleAt            int64 `json:"lastSampleAt"`
}

func canonicalEmbeddingShadowSlowDuration() time.Duration {
	return 900 * time.Millisecond
}

func normalizeCanonicalEmbeddingShadowSample(sample CanonicalEmbeddingShadowSample) (CanonicalEmbeddingShadowSample, error) {
	sample.UserID, sample.ProjectID = strings.TrimSpace(sample.UserID), strings.TrimSpace(sample.ProjectID)
	if sample.UserID == "" || sample.ProjectID == "" {
		return CanonicalEmbeddingShadowSample{}, errors.New("embedding shadow sample requires user and project")
	}
	if sample.SemanticHits < 0 || sample.HighSimilarityHits < 0 || sample.HighSimilarityHits > sample.SemanticHits {
		return CanonicalEmbeddingShadowSample{}, errors.New("embedding shadow sample hit counts are invalid")
	}
	if sample.DurationMillis < 0 {
		sample.DurationMillis = 0
	}
	if sample.DurationMillis > 30_000 {
		sample.DurationMillis = 30_000
	}
	return sample, nil
}

func (s *Store) RecordCanonicalEmbeddingShadowSample(ctx context.Context, sample CanonicalEmbeddingShadowSample) error {
	if s == nil || s.DB == nil || !canonicalEmbeddingEnabled() {
		return nil
	}
	normalized, err := normalizeCanonicalEmbeddingShadowSample(sample)
	if err != nil {
		return err
	}
	success, failure := int64(1), int64(0)
	if normalized.Errored {
		success, failure = 0, 1
	}
	nonempty := int64(0)
	if normalized.SemanticHits > 0 {
		nonempty = 1
	}
	slow := int64(0)
	if time.Duration(normalized.DurationMillis)*time.Millisecond >= canonicalEmbeddingShadowSlowDuration() {
		slow = 1
	}
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, upsertCanonicalEmbeddingShadowMetricsSQL,
		normalized.UserID, normalized.ProjectID, success, failure, nonempty, normalized.SemanticHits,
		normalized.HighSimilarityHits, slow, normalized.DurationMillis, now,
	)
	return err
}
