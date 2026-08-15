package cloud

import (
	"context"
	"errors"
	"strings"
	"time"
)

const canonicalSemanticCanaryMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_semantic_canary_metrics (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 attempts_total BIGINT NOT NULL DEFAULT 0,
 readiness_ready_count BIGINT NOT NULL DEFAULT 0,
 applied_count BIGINT NOT NULL DEFAULT 0,
 applied_claims_total BIGINT NOT NULL DEFAULT 0,
 deterministic_fallback_count BIGINT NOT NULL DEFAULT 0,
 readiness_blocked_count BIGINT NOT NULL DEFAULT 0,
 readiness_error_count BIGINT NOT NULL DEFAULT 0,
 recall_error_count BIGINT NOT NULL DEFAULT 0,
 no_high_similarity_count BIGINT NOT NULL DEFAULT 0,
 no_unique_claim_count BIGINT NOT NULL DEFAULT 0,
 timeout_count BIGINT NOT NULL DEFAULT 0,
 scope_unavailable_count BIGINT NOT NULL DEFAULT 0,
 slow_count BIGINT NOT NULL DEFAULT 0,
 duration_millis_total BIGINT NOT NULL DEFAULT 0,
 last_duration_millis BIGINT NOT NULL DEFAULT 0,
 max_duration_millis BIGINT NOT NULL DEFAULT 0,
 first_sample_at BIGINT NOT NULL,
 last_sample_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_semantic_canary_metrics_last
 ON codelocal_knowledge_semantic_canary_metrics(last_sample_at DESC);
`

const upsertCanonicalSemanticCanaryMetricsSQL = `
INSERT INTO codelocal_knowledge_semantic_canary_metrics(
 user_id,project_id,attempts_total,readiness_ready_count,applied_count,applied_claims_total,
 deterministic_fallback_count,readiness_blocked_count,readiness_error_count,recall_error_count,
 no_high_similarity_count,no_unique_claim_count,timeout_count,scope_unavailable_count,slow_count,
 duration_millis_total,last_duration_millis,max_duration_millis,first_sample_at,last_sample_at)
VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$15,$15,$16,$16)
ON CONFLICT(user_id,project_id) DO UPDATE SET
 attempts_total=codelocal_knowledge_semantic_canary_metrics.attempts_total+1,
 readiness_ready_count=codelocal_knowledge_semantic_canary_metrics.readiness_ready_count+EXCLUDED.readiness_ready_count,
 applied_count=codelocal_knowledge_semantic_canary_metrics.applied_count+EXCLUDED.applied_count,
 applied_claims_total=codelocal_knowledge_semantic_canary_metrics.applied_claims_total+EXCLUDED.applied_claims_total,
 deterministic_fallback_count=codelocal_knowledge_semantic_canary_metrics.deterministic_fallback_count+EXCLUDED.deterministic_fallback_count,
 readiness_blocked_count=codelocal_knowledge_semantic_canary_metrics.readiness_blocked_count+EXCLUDED.readiness_blocked_count,
 readiness_error_count=codelocal_knowledge_semantic_canary_metrics.readiness_error_count+EXCLUDED.readiness_error_count,
 recall_error_count=codelocal_knowledge_semantic_canary_metrics.recall_error_count+EXCLUDED.recall_error_count,
 no_high_similarity_count=codelocal_knowledge_semantic_canary_metrics.no_high_similarity_count+EXCLUDED.no_high_similarity_count,
 no_unique_claim_count=codelocal_knowledge_semantic_canary_metrics.no_unique_claim_count+EXCLUDED.no_unique_claim_count,
 timeout_count=codelocal_knowledge_semantic_canary_metrics.timeout_count+EXCLUDED.timeout_count,
 scope_unavailable_count=codelocal_knowledge_semantic_canary_metrics.scope_unavailable_count+EXCLUDED.scope_unavailable_count,
 slow_count=codelocal_knowledge_semantic_canary_metrics.slow_count+EXCLUDED.slow_count,
 duration_millis_total=codelocal_knowledge_semantic_canary_metrics.duration_millis_total+EXCLUDED.duration_millis_total,
 last_duration_millis=EXCLUDED.last_duration_millis,
 max_duration_millis=GREATEST(codelocal_knowledge_semantic_canary_metrics.max_duration_millis,EXCLUDED.max_duration_millis),
 last_sample_at=EXCLUDED.last_sample_at`

const canonicalSemanticCanarySummarySQL = `
SELECT
 COALESCE(SUM(attempts_total),0),COALESCE(SUM(readiness_ready_count),0),COALESCE(SUM(applied_count),0),
 COALESCE(SUM(applied_claims_total),0),COALESCE(SUM(deterministic_fallback_count),0),
 COALESCE(SUM(readiness_blocked_count),0),COALESCE(SUM(readiness_error_count),0),COALESCE(SUM(recall_error_count),0),
 COALESCE(SUM(no_high_similarity_count),0),COALESCE(SUM(no_unique_claim_count),0),COALESCE(SUM(timeout_count),0),
 COALESCE(SUM(scope_unavailable_count),0),COALESCE(SUM(slow_count),0),COALESCE(SUM(duration_millis_total),0),
 COALESCE(MAX(last_duration_millis),0),COALESCE(MAX(max_duration_millis),0),COALESCE(MIN(first_sample_at),0),COALESCE(MAX(last_sample_at),0),
 COUNT(*)::bigint
FROM codelocal_knowledge_semantic_canary_metrics
WHERE ($1='' OR user_id=$1)`

const (
	SemanticCanaryReasonReady            = "semantic_hybrid_ready"
	SemanticCanaryReasonNotReady         = "semantic_hybrid_not_ready"
	SemanticCanaryReasonReadinessError   = "semantic_hybrid_readiness_error"
	SemanticCanaryReasonRecallError      = "semantic_hybrid_recall_error"
	SemanticCanaryReasonNoSimilarity     = "semantic_hybrid_no_high_similarity_hits"
	SemanticCanaryReasonNoUniqueClaims   = "semantic_hybrid_no_unique_claims"
	SemanticCanaryReasonTimeout          = "semantic_hybrid_timeout"
	SemanticCanaryReasonScopeUnavailable = "semantic_hybrid_scope_unavailable"
)

type CanonicalSemanticCanarySample struct {
	UserID         string
	ProjectID      string
	Reason         string
	DurationMillis int64
	AppliedClaims  int
}

type CanonicalSemanticCanaryMetrics struct {
	AttemptsTotal              int64 `json:"attemptsTotal"`
	ReadinessReadyCount        int64 `json:"readinessReadyCount"`
	AppliedCount               int64 `json:"appliedCount"`
	AppliedClaimsTotal         int64 `json:"appliedClaimsTotal"`
	DeterministicFallbackCount int64 `json:"deterministicFallbackCount"`
	ReadinessBlockedCount      int64 `json:"readinessBlockedCount"`
	ReadinessErrorCount        int64 `json:"readinessErrorCount"`
	RecallErrorCount           int64 `json:"recallErrorCount"`
	NoHighSimilarityCount      int64 `json:"noHighSimilarityCount"`
	NoUniqueClaimCount         int64 `json:"noUniqueClaimCount"`
	TimeoutCount               int64 `json:"timeoutCount"`
	ScopeUnavailableCount      int64 `json:"scopeUnavailableCount"`
	SlowCount                  int64 `json:"slowCount"`
	DurationMillisTotal        int64 `json:"durationMillisTotal"`
	LastDurationMillis         int64 `json:"lastDurationMillis"`
	MaxDurationMillis          int64 `json:"maxDurationMillis"`
	FirstSampleAt              int64 `json:"firstSampleAt"`
	LastSampleAt               int64 `json:"lastSampleAt"`
	ProjectCount               int64 `json:"projectCount"`
}

type canonicalSemanticCanaryBuckets struct {
	readinessReady, applied, fallback, readinessBlocked, readinessError int64
	recallError, noSimilarity, noUnique, timeout, scopeUnavailable      int64
}

func canonicalSemanticCanaryBucketsFor(reason string, appliedClaims int) (canonicalSemanticCanaryBuckets, error) {
	b := canonicalSemanticCanaryBuckets{}
	switch strings.TrimSpace(reason) {
	case SemanticCanaryReasonReady:
		b.readinessReady, b.applied = 1, 1
	case SemanticCanaryReasonNotReady:
		b.readinessBlocked, b.fallback = 1, 1
	case SemanticCanaryReasonReadinessError:
		b.readinessError, b.fallback = 1, 1
	case SemanticCanaryReasonRecallError:
		b.readinessReady, b.recallError, b.fallback = 1, 1, 1
	case SemanticCanaryReasonNoSimilarity:
		b.readinessReady, b.noSimilarity, b.fallback = 1, 1, 1
	case SemanticCanaryReasonNoUniqueClaims:
		b.readinessReady, b.noUnique, b.fallback = 1, 1, 1
	case SemanticCanaryReasonTimeout:
		b.timeout, b.fallback = 1, 1
	case SemanticCanaryReasonScopeUnavailable:
		b.scopeUnavailable, b.fallback = 1, 1
	default:
		return canonicalSemanticCanaryBuckets{}, errors.New("unknown semantic canary reason")
	}
	if b.applied == 1 && (appliedClaims < 1 || appliedClaims > 2) {
		return canonicalSemanticCanaryBuckets{}, errors.New("applied semantic canary sample requires one or two claims")
	}
	if b.applied == 0 && appliedClaims != 0 {
		return canonicalSemanticCanaryBuckets{}, errors.New("fallback semantic canary sample cannot report applied claims")
	}
	return b, nil
}

func normalizeCanonicalSemanticCanarySample(sample CanonicalSemanticCanarySample) (CanonicalSemanticCanarySample, canonicalSemanticCanaryBuckets, error) {
	sample.UserID, sample.ProjectID = strings.TrimSpace(sample.UserID), strings.TrimSpace(sample.ProjectID)
	if sample.UserID == "" || sample.ProjectID == "" {
		return CanonicalSemanticCanarySample{}, canonicalSemanticCanaryBuckets{}, errors.New("semantic canary sample requires user and project")
	}
	if sample.DurationMillis < 0 {
		sample.DurationMillis = 0
	}
	if sample.DurationMillis > 30_000 {
		sample.DurationMillis = 30_000
	}
	buckets, err := canonicalSemanticCanaryBucketsFor(sample.Reason, sample.AppliedClaims)
	if err != nil {
		return CanonicalSemanticCanarySample{}, canonicalSemanticCanaryBuckets{}, err
	}
	return sample, buckets, nil
}

func (s *Store) RecordCanonicalSemanticCanarySample(ctx context.Context, sample CanonicalSemanticCanarySample) error {
	if s == nil || s.DB == nil {
		return nil
	}
	normalized, buckets, err := normalizeCanonicalSemanticCanarySample(sample)
	if err != nil {
		return err
	}
	slow := int64(0)
	if time.Duration(normalized.DurationMillis)*time.Millisecond >= CanonicalEmbeddingShadowSlowDuration {
		slow = 1
	}
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, upsertCanonicalSemanticCanaryMetricsSQL,
		normalized.UserID, normalized.ProjectID, buckets.readinessReady, buckets.applied, normalized.AppliedClaims,
		buckets.fallback, buckets.readinessBlocked, buckets.readinessError, buckets.recallError, buckets.noSimilarity,
		buckets.noUnique, buckets.timeout, buckets.scopeUnavailable, slow, normalized.DurationMillis, now,
	)
	return err
}

func scanCanonicalSemanticCanaryMetrics(row interface{ Scan(...any) error }) (CanonicalSemanticCanaryMetrics, error) {
	var metrics CanonicalSemanticCanaryMetrics
	err := row.Scan(
		&metrics.AttemptsTotal, &metrics.ReadinessReadyCount, &metrics.AppliedCount, &metrics.AppliedClaimsTotal,
		&metrics.DeterministicFallbackCount, &metrics.ReadinessBlockedCount, &metrics.ReadinessErrorCount, &metrics.RecallErrorCount,
		&metrics.NoHighSimilarityCount, &metrics.NoUniqueClaimCount, &metrics.TimeoutCount, &metrics.ScopeUnavailableCount,
		&metrics.SlowCount, &metrics.DurationMillisTotal, &metrics.LastDurationMillis, &metrics.MaxDurationMillis,
		&metrics.FirstSampleAt, &metrics.LastSampleAt, &metrics.ProjectCount,
	)
	return metrics, err
}

func (s *Store) CanonicalSemanticCanaryMetrics(ctx context.Context, userID string) (CanonicalSemanticCanaryMetrics, error) {
	if s == nil || s.DB == nil {
		return CanonicalSemanticCanaryMetrics{}, errors.New("semantic canary metrics unavailable")
	}
	return scanCanonicalSemanticCanaryMetrics(s.DB.QueryRow(ctx, canonicalSemanticCanarySummarySQL, strings.TrimSpace(userID)))
}
