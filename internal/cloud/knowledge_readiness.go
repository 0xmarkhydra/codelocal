package cloud

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const knowledgeV2ReadinessMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_shadow_metrics (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 samples_total BIGINT NOT NULL DEFAULT 0 CHECK (samples_total >= 0),
 success_count BIGINT NOT NULL DEFAULT 0 CHECK (success_count >= 0),
 error_count BIGINT NOT NULL DEFAULT 0 CHECK (error_count >= 0),
 legacy_nonempty_samples BIGINT NOT NULL DEFAULT 0 CHECK (legacy_nonempty_samples >= 0),
 canonical_nonempty_samples BIGINT NOT NULL DEFAULT 0 CHECK (canonical_nonempty_samples >= 0),
 legacy_hits_total BIGINT NOT NULL DEFAULT 0 CHECK (legacy_hits_total >= 0),
 canonical_hits_total BIGINT NOT NULL DEFAULT 0 CHECK (canonical_hits_total >= 0),
 high_confidence_hits_total BIGINT NOT NULL DEFAULT 0 CHECK (high_confidence_hits_total >= 0),
 slow_samples BIGINT NOT NULL DEFAULT 0 CHECK (slow_samples >= 0),
 duration_ms_total BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms_total >= 0),
 last_duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (last_duration_ms >= 0),
 max_duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (max_duration_ms >= 0),
 first_sample_at BIGINT NOT NULL,
 last_sample_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_shadow_metrics_recent
 ON codelocal_knowledge_shadow_metrics(last_sample_at DESC);
`

const upsertKnowledgeV2ShadowMetricsSQL = `
INSERT INTO codelocal_knowledge_shadow_metrics(
 user_id,project_id,samples_total,success_count,error_count,legacy_nonempty_samples,canonical_nonempty_samples,
 legacy_hits_total,canonical_hits_total,high_confidence_hits_total,slow_samples,duration_ms_total,last_duration_ms,max_duration_ms,
 first_sample_at,last_sample_at)
VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12,$13,$13)
ON CONFLICT(user_id,project_id) DO UPDATE SET
 samples_total=codelocal_knowledge_shadow_metrics.samples_total+1,
 success_count=codelocal_knowledge_shadow_metrics.success_count+EXCLUDED.success_count,
 error_count=codelocal_knowledge_shadow_metrics.error_count+EXCLUDED.error_count,
 legacy_nonempty_samples=codelocal_knowledge_shadow_metrics.legacy_nonempty_samples+EXCLUDED.legacy_nonempty_samples,
 canonical_nonempty_samples=codelocal_knowledge_shadow_metrics.canonical_nonempty_samples+EXCLUDED.canonical_nonempty_samples,
 legacy_hits_total=codelocal_knowledge_shadow_metrics.legacy_hits_total+EXCLUDED.legacy_hits_total,
 canonical_hits_total=codelocal_knowledge_shadow_metrics.canonical_hits_total+EXCLUDED.canonical_hits_total,
 high_confidence_hits_total=codelocal_knowledge_shadow_metrics.high_confidence_hits_total+EXCLUDED.high_confidence_hits_total,
 slow_samples=codelocal_knowledge_shadow_metrics.slow_samples+EXCLUDED.slow_samples,
 duration_ms_total=codelocal_knowledge_shadow_metrics.duration_ms_total+EXCLUDED.duration_ms_total,
 last_duration_ms=EXCLUDED.last_duration_ms,
 max_duration_ms=GREATEST(codelocal_knowledge_shadow_metrics.max_duration_ms,EXCLUDED.max_duration_ms),
 last_sample_at=GREATEST(codelocal_knowledge_shadow_metrics.last_sample_at,EXCLUDED.last_sample_at)`

const knowledgeV2ShadowMetricsSelectSQL = `
SELECT samples_total,success_count,error_count,legacy_nonempty_samples,canonical_nonempty_samples,
 legacy_hits_total,canonical_hits_total,high_confidence_hits_total,slow_samples,duration_ms_total,last_duration_ms,max_duration_ms,
 first_sample_at,last_sample_at
FROM codelocal_knowledge_shadow_metrics
WHERE user_id=$1 AND project_id=$2`

const knowledgeV2ReadinessHealthSQL = `
SELECT status,auto_promotion_enabled,evaluated_at
FROM codelocal_knowledge_health
WHERE user_id=$1 AND project_id=$2`

const knowledgeV2ShadowSlowDuration = 350 * time.Millisecond

type KnowledgeV2ShadowSample struct {
	UserID             string
	ProjectID          string
	LegacyCount        int
	CanonicalCount     int
	HighConfidenceHits int
	DurationMillis     int64
	Errored            bool
}

type KnowledgeV2ShadowMetrics struct {
	SamplesTotal             int64 `json:"samplesTotal"`
	SuccessCount             int64 `json:"successCount"`
	ErrorCount               int64 `json:"errorCount"`
	LegacyNonemptySamples    int64 `json:"legacyNonemptySamples"`
	CanonicalNonemptySamples int64 `json:"canonicalNonemptySamples"`
	LegacyHitsTotal          int64 `json:"legacyHitsTotal"`
	CanonicalHitsTotal       int64 `json:"canonicalHitsTotal"`
	HighConfidenceHitsTotal  int64 `json:"highConfidenceHitsTotal"`
	SlowSamples              int64 `json:"slowSamples"`
	DurationMillisTotal      int64 `json:"durationMillisTotal"`
	LastDurationMillis       int64 `json:"lastDurationMillis"`
	MaxDurationMillis        int64 `json:"maxDurationMillis"`
	FirstSampleAt            int64 `json:"firstSampleAt"`
	LastSampleAt             int64 `json:"lastSampleAt"`
}

type KnowledgeV2Readiness struct {
	UserID                string   `json:"userId"`
	ProjectID             string   `json:"projectId"`
	Status                string   `json:"status"`
	ReasonCodes           []string `json:"reasonCodes"`
	SamplesTotal          int64    `json:"samplesTotal"`
	ErrorRatePercent      float64  `json:"errorRatePercent"`
	SlowRatePercent       float64  `json:"slowRatePercent"`
	HighConfidencePercent float64  `json:"highConfidencePercent"`
	CanonicalHitsTotal    int64    `json:"canonicalHitsTotal"`
	HealthStatus          string   `json:"healthStatus"`
	LastSampleAt          int64    `json:"lastSampleAt"`
}

type knowledgeV2ReadinessThresholds struct {
	MinSamples               int64
	MaxErrorPercent          float64
	MaxSlowPercent           float64
	MinCanonicalObservations int64
	MinHighConfidencePercent float64
	Freshness                time.Duration
}

func normalizeKnowledgeV2ShadowSample(sample KnowledgeV2ShadowSample) (KnowledgeV2ShadowSample, error) {
	sample.UserID = strings.TrimSpace(sample.UserID)
	sample.ProjectID = strings.TrimSpace(sample.ProjectID)
	if sample.UserID == "" || sample.ProjectID == "" {
		return KnowledgeV2ShadowSample{}, errors.New("knowledge v2 shadow sample requires user and project")
	}
	if sample.LegacyCount < 0 {
		sample.LegacyCount = 0
	}
	if sample.CanonicalCount < 0 {
		sample.CanonicalCount = 0
	}
	if sample.HighConfidenceHits < 0 {
		sample.HighConfidenceHits = 0
	}
	if sample.HighConfidenceHits > sample.CanonicalCount {
		sample.HighConfidenceHits = sample.CanonicalCount
	}
	if sample.DurationMillis < 0 {
		sample.DurationMillis = 0
	}
	if sample.DurationMillis > 60_000 {
		sample.DurationMillis = 60_000
	}
	if sample.Errored {
		sample.CanonicalCount = 0
		sample.HighConfidenceHits = 0
	}
	return sample, nil
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) RecordKnowledgeV2ShadowSample(ctx context.Context, sample KnowledgeV2ShadowSample) error {
	if s == nil || s.DB == nil {
		return errors.New("knowledge v2 shadow metrics unavailable")
	}
	normalized, err := normalizeKnowledgeV2ShadowSample(sample)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	success := !normalized.Errored
	slow := time.Duration(normalized.DurationMillis)*time.Millisecond >= knowledgeV2ShadowSlowDuration
	_, err = s.DB.Exec(ctx, upsertKnowledgeV2ShadowMetricsSQL,
		normalized.UserID, normalized.ProjectID,
		boolCount(success), boolCount(normalized.Errored), boolCount(normalized.LegacyCount > 0), boolCount(success && normalized.CanonicalCount > 0),
		normalized.LegacyCount, normalized.CanonicalCount, normalized.HighConfidenceHits, boolCount(slow), normalized.DurationMillis, normalized.DurationMillis, now,
	)
	return err
}

func percent(numerator, denominator int64) float64 {
	if denominator <= 0 || numerator <= 0 {
		return 0
	}
	return float64(numerator) * 100 / float64(denominator)
}

func knowledgeV2ReadinessThresholdConfig() knowledgeV2ReadinessThresholds {
	minSamples := envInt("CODELOCAL_KNOWLEDGE_V2_READY_MIN_SAMPLES", 50)
	if minSamples < 10 {
		minSamples = 10
	}
	if minSamples > 1000 {
		minSamples = 1000
	}
	maxError := envInt("CODELOCAL_KNOWLEDGE_V2_READY_MAX_ERROR_PERCENT", 2)
	if maxError < 0 {
		maxError = 0
	}
	if maxError > 25 {
		maxError = 25
	}
	maxSlow := envInt("CODELOCAL_KNOWLEDGE_V2_READY_MAX_SLOW_PERCENT", 10)
	if maxSlow < 0 {
		maxSlow = 0
	}
	if maxSlow > 50 {
		maxSlow = 50
	}
	minCanonical := envInt("CODELOCAL_KNOWLEDGE_V2_READY_MIN_CANONICAL_HITS", 10)
	if minCanonical < 1 {
		minCanonical = 1
	}
	if minCanonical > 5000 {
		minCanonical = 5000
	}
	minHighConfidence := envInt("CODELOCAL_KNOWLEDGE_V2_READY_MIN_HIGH_CONFIDENCE_PERCENT", 80)
	if minHighConfidence < 50 {
		minHighConfidence = 50
	}
	if minHighConfidence > 100 {
		minHighConfidence = 100
	}
	freshnessDays := envInt("CODELOCAL_KNOWLEDGE_V2_READY_FRESH_DAYS", 7)
	if freshnessDays < 1 {
		freshnessDays = 1
	}
	if freshnessDays > 30 {
		freshnessDays = 30
	}
	return knowledgeV2ReadinessThresholds{
		MinSamples: int64(minSamples), MaxErrorPercent: float64(maxError), MaxSlowPercent: float64(maxSlow),
		MinCanonicalObservations: int64(minCanonical), MinHighConfidencePercent: float64(minHighConfidence),
		Freshness: time.Duration(freshnessDays) * 24 * time.Hour,
	}
}

func knowledgeV2ReadinessFrom(userID, projectID string, metrics *KnowledgeV2ShadowMetrics, health *KnowledgeHealth, thresholds knowledgeV2ReadinessThresholds, now time.Time) KnowledgeV2Readiness {
	readiness := KnowledgeV2Readiness{UserID: userID, ProjectID: projectID, Status: "collecting"}
	if health == nil {
		readiness.ReasonCodes = []string{"knowledge_health_missing"}
		return readiness
	}
	readiness.HealthStatus = health.Status
	if health.Status == knowledgeHealthCritical || !health.AutoPromotionEnabled {
		readiness.Status = "blocked"
		readiness.ReasonCodes = []string{"knowledge_health_critical"}
		return readiness
	}
	if health.Status != knowledgeHealthHealthy {
		readiness.ReasonCodes = []string{"knowledge_health_not_healthy"}
		return readiness
	}
	if metrics == nil {
		readiness.ReasonCodes = []string{"shadow_metrics_missing"}
		return readiness
	}
	readiness.SamplesTotal = metrics.SamplesTotal
	readiness.CanonicalHitsTotal = metrics.CanonicalHitsTotal
	readiness.LastSampleAt = metrics.LastSampleAt
	readiness.ErrorRatePercent = percent(metrics.ErrorCount, metrics.SamplesTotal)
	readiness.SlowRatePercent = percent(metrics.SlowSamples, metrics.SamplesTotal)
	readiness.HighConfidencePercent = percent(metrics.HighConfidenceHitsTotal, metrics.CanonicalHitsTotal)
	if metrics.LastSampleAt <= 0 || now.Sub(time.UnixMilli(metrics.LastSampleAt)) > thresholds.Freshness {
		readiness.ReasonCodes = []string{"shadow_metrics_stale"}
		return readiness
	}
	if metrics.SamplesTotal < thresholds.MinSamples {
		readiness.ReasonCodes = []string{"insufficient_shadow_samples"}
		return readiness
	}
	blocked := []string{}
	if readiness.ErrorRatePercent > thresholds.MaxErrorPercent {
		blocked = append(blocked, "shadow_error_rate_high")
	}
	if readiness.SlowRatePercent > thresholds.MaxSlowPercent {
		blocked = append(blocked, "shadow_slow_rate_high")
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		readiness.Status = "blocked"
		readiness.ReasonCodes = blocked
		return readiness
	}
	if metrics.CanonicalHitsTotal < thresholds.MinCanonicalObservations {
		readiness.ReasonCodes = []string{"insufficient_canonical_observations"}
		return readiness
	}
	if readiness.HighConfidencePercent < thresholds.MinHighConfidencePercent {
		readiness.Status = "blocked"
		readiness.ReasonCodes = []string{"canonical_confidence_ratio_low"}
		return readiness
	}
	readiness.Status = "ready"
	readiness.ReasonCodes = []string{"shadow_gate_passed"}
	return readiness
}

func scanKnowledgeV2ShadowMetrics(row interface{ Scan(...any) error }) (KnowledgeV2ShadowMetrics, error) {
	var metrics KnowledgeV2ShadowMetrics
	err := row.Scan(
		&metrics.SamplesTotal, &metrics.SuccessCount, &metrics.ErrorCount, &metrics.LegacyNonemptySamples, &metrics.CanonicalNonemptySamples,
		&metrics.LegacyHitsTotal, &metrics.CanonicalHitsTotal, &metrics.HighConfidenceHitsTotal, &metrics.SlowSamples, &metrics.DurationMillisTotal,
		&metrics.LastDurationMillis, &metrics.MaxDurationMillis, &metrics.FirstSampleAt, &metrics.LastSampleAt,
	)
	return metrics, err
}

func (s *Store) KnowledgeV2Readiness(ctx context.Context, userID, projectID string) (KnowledgeV2Readiness, error) {
	if s == nil || s.DB == nil {
		return KnowledgeV2Readiness{}, errors.New("knowledge v2 readiness unavailable")
	}
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return KnowledgeV2Readiness{}, errors.New("knowledge v2 readiness requires user and project")
	}
	var health KnowledgeHealth
	healthErr := s.DB.QueryRow(ctx, knowledgeV2ReadinessHealthSQL, userID, projectID).Scan(&health.Status, &health.AutoPromotionEnabled, &health.EvaluatedAt)
	var healthPtr *KnowledgeHealth
	if healthErr == nil {
		health.UserID, health.ProjectID = userID, projectID
		healthPtr = &health
	} else if !errors.Is(healthErr, pgx.ErrNoRows) {
		return KnowledgeV2Readiness{}, healthErr
	}
	metrics, metricsErr := scanKnowledgeV2ShadowMetrics(s.DB.QueryRow(ctx, knowledgeV2ShadowMetricsSelectSQL, userID, projectID))
	var metricsPtr *KnowledgeV2ShadowMetrics
	if metricsErr == nil {
		metricsPtr = &metrics
	} else if !errors.Is(metricsErr, pgx.ErrNoRows) {
		return KnowledgeV2Readiness{}, metricsErr
	}
	return knowledgeV2ReadinessFrom(userID, projectID, metricsPtr, healthPtr, knowledgeV2ReadinessThresholdConfig(), time.Now()), nil
}
