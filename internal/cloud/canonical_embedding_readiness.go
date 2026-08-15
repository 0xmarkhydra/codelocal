package cloud

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const canonicalEmbeddingShadowMetricsSelectSQL = `
SELECT samples_total,success_count,error_count,semantic_nonempty_samples,semantic_hits_total,
 high_similarity_hits_total,slow_samples,duration_millis_total,last_duration_millis,max_duration_millis,
 first_sample_at,last_sample_at
FROM codelocal_knowledge_embedding_shadow_metrics
WHERE user_id=$1 AND project_id=$2`

type CanonicalSemanticReadiness struct {
	UserID                string   `json:"userId"`
	ProjectID             string   `json:"projectId"`
	Status                string   `json:"status"`
	ReasonCodes           []string `json:"reasonCodes"`
	SamplesTotal          int64    `json:"samplesTotal"`
	SemanticHitsTotal     int64    `json:"semanticHitsTotal"`
	ErrorRatePercent      float64  `json:"errorRatePercent"`
	SlowRatePercent       float64  `json:"slowRatePercent"`
	HighSimilarityPercent float64  `json:"highSimilarityPercent"`
	LastSampleAt          int64    `json:"lastSampleAt"`
	BaseReadinessStatus   string   `json:"baseReadinessStatus"`
	EmbeddingStatus       string   `json:"embeddingStatus"`
}

type canonicalSemanticReadinessThresholds struct {
	MinSamples               int64
	MaxErrorPercent          float64
	MaxSlowPercent           float64
	MinSemanticHits          int64
	MinHighSimilarityPercent float64
	Freshness                time.Duration
}

func canonicalSemanticReadinessThresholdConfig() canonicalSemanticReadinessThresholds {
	minSamples := clampEnvInt("CODELOCAL_CANONICAL_SEMANTIC_READY_MIN_SAMPLES", 50, 10, 1000)
	maxError := clampEnvInt("CODELOCAL_CANONICAL_SEMANTIC_READY_MAX_ERROR_PERCENT", 2, 0, 25)
	maxSlow := clampEnvInt("CODELOCAL_CANONICAL_SEMANTIC_READY_MAX_SLOW_PERCENT", 10, 0, 50)
	minHits := clampEnvInt("CODELOCAL_CANONICAL_SEMANTIC_READY_MIN_HITS", 20, 1, 5000)
	minSimilarity := clampEnvInt("CODELOCAL_CANONICAL_SEMANTIC_READY_MIN_HIGH_SIMILARITY_PERCENT", 60, 40, 100)
	freshDays := clampEnvInt("CODELOCAL_CANONICAL_SEMANTIC_READY_FRESH_DAYS", 7, 1, 30)
	return canonicalSemanticReadinessThresholds{
		MinSamples: int64(minSamples), MaxErrorPercent: float64(maxError), MaxSlowPercent: float64(maxSlow),
		MinSemanticHits: int64(minHits), MinHighSimilarityPercent: float64(minSimilarity), Freshness: time.Duration(freshDays) * 24 * time.Hour,
	}
}

func clampEnvInt(key string, fallback, minValue, maxValue int) int {
	value := envInt(key, fallback)
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func canonicalSemanticReadinessFrom(userID, projectID string, metrics *CanonicalEmbeddingShadowMetrics, base KnowledgeV2Readiness, embedding CanonicalEmbeddingFreshness, thresholds canonicalSemanticReadinessThresholds, now time.Time) CanonicalSemanticReadiness {
	readiness := CanonicalSemanticReadiness{UserID: userID, ProjectID: projectID, Status: "collecting", BaseReadinessStatus: base.Status, EmbeddingStatus: embedding.Status}
	if base.Status != "ready" {
		readiness.ReasonCodes = []string{"knowledge_v2_not_ready"}
		if base.Status == "blocked" {
			readiness.Status = "blocked"
		}
		return readiness
	}
	if embedding.Status != "current" && embedding.Status != "empty" {
		readiness.ReasonCodes = []string{"embedding_index_not_current"}
		return readiness
	}
	if embedding.Status == "empty" {
		readiness.ReasonCodes = []string{"embedding_index_empty"}
		return readiness
	}
	if metrics == nil {
		readiness.ReasonCodes = []string{"semantic_shadow_metrics_missing"}
		return readiness
	}
	readiness.SamplesTotal, readiness.SemanticHitsTotal, readiness.LastSampleAt = metrics.SamplesTotal, metrics.SemanticHitsTotal, metrics.LastSampleAt
	readiness.ErrorRatePercent = percent(metrics.ErrorCount, metrics.SamplesTotal)
	readiness.SlowRatePercent = percent(metrics.SlowSamples, metrics.SamplesTotal)
	readiness.HighSimilarityPercent = percent(metrics.HighSimilarityHitsTotal, metrics.SemanticHitsTotal)
	if metrics.LastSampleAt <= 0 || now.Sub(time.UnixMilli(metrics.LastSampleAt)) > thresholds.Freshness {
		readiness.ReasonCodes = []string{"semantic_shadow_metrics_stale"}
		return readiness
	}
	if metrics.SamplesTotal < thresholds.MinSamples {
		readiness.ReasonCodes = []string{"insufficient_semantic_shadow_samples"}
		return readiness
	}
	blocked := []string{}
	if readiness.ErrorRatePercent > thresholds.MaxErrorPercent {
		blocked = append(blocked, "semantic_shadow_error_rate_high")
	}
	if readiness.SlowRatePercent > thresholds.MaxSlowPercent {
		blocked = append(blocked, "semantic_shadow_slow_rate_high")
	}
	if readiness.HighSimilarityPercent < thresholds.MinHighSimilarityPercent && metrics.SemanticHitsTotal >= thresholds.MinSemanticHits {
		blocked = append(blocked, "semantic_similarity_ratio_low")
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		readiness.Status, readiness.ReasonCodes = "blocked", blocked
		return readiness
	}
	if metrics.SemanticHitsTotal < thresholds.MinSemanticHits {
		readiness.ReasonCodes = []string{"insufficient_semantic_hits"}
		return readiness
	}
	readiness.Status, readiness.ReasonCodes = "ready", []string{"semantic_shadow_gate_passed"}
	return readiness
}

func scanCanonicalEmbeddingShadowMetrics(row interface{ Scan(...any) error }) (CanonicalEmbeddingShadowMetrics, error) {
	var metrics CanonicalEmbeddingShadowMetrics
	err := row.Scan(&metrics.SamplesTotal, &metrics.SuccessCount, &metrics.ErrorCount, &metrics.SemanticNonemptySamples, &metrics.SemanticHitsTotal,
		&metrics.HighSimilarityHitsTotal, &metrics.SlowSamples, &metrics.DurationMillisTotal, &metrics.LastDurationMillis, &metrics.MaxDurationMillis,
		&metrics.FirstSampleAt, &metrics.LastSampleAt)
	return metrics, err
}

func (s *Store) CanonicalSemanticReadiness(ctx context.Context, userID, projectID string) (CanonicalSemanticReadiness, error) {
	if s == nil || s.DB == nil {
		return CanonicalSemanticReadiness{}, errors.New("canonical semantic readiness unavailable")
	}
	userID, projectID = strings.TrimSpace(userID), strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return CanonicalSemanticReadiness{}, errors.New("canonical semantic readiness requires user and project")
	}
	base, err := s.KnowledgeV2Readiness(ctx, userID, projectID)
	if err != nil {
		return CanonicalSemanticReadiness{}, err
	}
	embedding, embeddingErr := s.CanonicalEmbeddingProjectFreshness(ctx, userID, projectID)
	if embeddingErr != nil {
		embedding = CanonicalEmbeddingFreshness{UserID: userID, ProjectID: projectID, Status: "unavailable"}
	}
	metrics, metricsErr := scanCanonicalEmbeddingShadowMetrics(s.DB.QueryRow(ctx, canonicalEmbeddingShadowMetricsSelectSQL, userID, projectID))
	var metricsPtr *CanonicalEmbeddingShadowMetrics
	if metricsErr == nil {
		metricsPtr = &metrics
	} else if !errors.Is(metricsErr, pgx.ErrNoRows) {
		return CanonicalSemanticReadiness{}, metricsErr
	}
	return canonicalSemanticReadinessFrom(userID, projectID, metricsPtr, base, embedding, canonicalSemanticReadinessThresholdConfig(), time.Now()), nil
}
