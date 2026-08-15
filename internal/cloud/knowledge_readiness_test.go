package cloud

import (
	"strings"
	"testing"
	"time"
)

func readinessThresholdFixture() knowledgeV2ReadinessThresholds {
	return knowledgeV2ReadinessThresholds{
		MinSamples: 10, MaxErrorPercent: 2, MaxSlowPercent: 10,
		MinCanonicalObservations: 5, MinHighConfidencePercent: 80, Freshness: 7 * 24 * time.Hour,
	}
}

func healthyReadinessHealth() *KnowledgeHealth {
	return &KnowledgeHealth{Status: knowledgeHealthHealthy, AutoPromotionEnabled: true}
}

func readyShadowMetrics(now time.Time) *KnowledgeV2ShadowMetrics {
	return &KnowledgeV2ShadowMetrics{
		SamplesTotal: 100, SuccessCount: 100, ErrorCount: 0,
		LegacyNonemptySamples: 90, CanonicalNonemptySamples: 60,
		LegacyHitsTotal: 300, CanonicalHitsTotal: 100, HighConfidenceHitsTotal: 90,
		SlowSamples: 5, DurationMillisTotal: 10000, LastDurationMillis: 50, MaxDurationMillis: 300,
		FirstSampleAt: now.Add(-time.Hour).UnixMilli(), LastSampleAt: now.UnixMilli(),
	}
}

func TestKnowledgeV2ReadinessCriticalHealthAlwaysBlocks(t *testing.T) {
	now := time.Now()
	health := &KnowledgeHealth{Status: knowledgeHealthCritical, AutoPromotionEnabled: false}
	readiness := knowledgeV2ReadinessFrom("user-a", "project-a", readyShadowMetrics(now), health, readinessThresholdFixture(), now)
	if readiness.Status != "blocked" || strings.Join(readiness.ReasonCodes, ",") != "knowledge_health_critical" {
		t.Fatalf("critical health did not dominate rollout readiness: %#v", readiness)
	}
}

func TestKnowledgeV2ReadinessCollectsUntilEvidenceIsSufficient(t *testing.T) {
	now := time.Now()
	thresholds := readinessThresholdFixture()
	for name, tc := range map[string]struct {
		metrics *KnowledgeV2ShadowMetrics
		health  *KnowledgeHealth
		reason  string
	}{
		"missing-health":     {readyShadowMetrics(now), nil, "knowledge_health_missing"},
		"degraded-health":    {readyShadowMetrics(now), &KnowledgeHealth{Status: knowledgeHealthDegraded, AutoPromotionEnabled: true}, "knowledge_health_not_healthy"},
		"missing-metrics":    {nil, healthyReadinessHealth(), "shadow_metrics_missing"},
		"few-samples":        {&KnowledgeV2ShadowMetrics{SamplesTotal: 9, LastSampleAt: now.UnixMilli()}, healthyReadinessHealth(), "insufficient_shadow_samples"},
		"few-canonical-hits": {&KnowledgeV2ShadowMetrics{SamplesTotal: 10, SuccessCount: 10, CanonicalHitsTotal: 4, HighConfidenceHitsTotal: 4, LastSampleAt: now.UnixMilli()}, healthyReadinessHealth(), "insufficient_canonical_observations"},
		"stale":              {&KnowledgeV2ShadowMetrics{SamplesTotal: 100, CanonicalHitsTotal: 20, HighConfidenceHitsTotal: 20, LastSampleAt: now.Add(-8 * 24 * time.Hour).UnixMilli()}, healthyReadinessHealth(), "shadow_metrics_stale"},
	} {
		t.Run(name, func(t *testing.T) {
			readiness := knowledgeV2ReadinessFrom("user-a", "project-a", tc.metrics, tc.health, thresholds, now)
			if readiness.Status != "collecting" || strings.Join(readiness.ReasonCodes, ",") != tc.reason {
				t.Fatalf("unexpected collecting gate: %#v", readiness)
			}
		})
	}
}

func TestKnowledgeV2ReadinessBlocksErrorSlowAndLowConfidenceRates(t *testing.T) {
	now := time.Now()
	thresholds := readinessThresholdFixture()
	metrics := readyShadowMetrics(now)
	metrics.ErrorCount = 3
	metrics.SuccessCount = 97
	metrics.SlowSamples = 11
	readiness := knowledgeV2ReadinessFrom("user-a", "project-a", metrics, healthyReadinessHealth(), thresholds, now)
	if readiness.Status != "blocked" || strings.Join(readiness.ReasonCodes, ",") != "shadow_error_rate_high,shadow_slow_rate_high" {
		t.Fatalf("error/slow threshold did not block readiness deterministically: %#v", readiness)
	}

	metrics = readyShadowMetrics(now)
	metrics.HighConfidenceHitsTotal = 79
	readiness = knowledgeV2ReadinessFrom("user-a", "project-a", metrics, healthyReadinessHealth(), thresholds, now)
	if readiness.Status != "blocked" || strings.Join(readiness.ReasonCodes, ",") != "canonical_confidence_ratio_low" {
		t.Fatalf("low-confidence canonical observations did not block readiness: %#v", readiness)
	}
}

func TestKnowledgeV2ReadinessPassesOnlyAfterAllGates(t *testing.T) {
	now := time.Now()
	readiness := knowledgeV2ReadinessFrom("user-a", "project-a", readyShadowMetrics(now), healthyReadinessHealth(), readinessThresholdFixture(), now)
	if readiness.Status != "ready" || strings.Join(readiness.ReasonCodes, ",") != "shadow_gate_passed" {
		t.Fatalf("sufficient healthy shadow evidence did not become ready: %#v", readiness)
	}
	if readiness.ErrorRatePercent != 0 || readiness.SlowRatePercent != 5 || readiness.HighConfidencePercent != 90 {
		t.Fatalf("readiness ratios are incorrect: %#v", readiness)
	}
}

func TestKnowledgeV2ShadowSampleNormalizationIsBounded(t *testing.T) {
	sample, err := normalizeKnowledgeV2ShadowSample(KnowledgeV2ShadowSample{
		UserID: " user-a ", ProjectID: " project-a ", LegacyCount: -4, CanonicalCount: 2,
		HighConfidenceHits: 99, DurationMillis: 999999, Errored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sample.UserID != "user-a" || sample.ProjectID != "project-a" || sample.LegacyCount != 0 || sample.CanonicalCount != 0 || sample.HighConfidenceHits != 0 || sample.DurationMillis != 60000 {
		t.Fatalf("shadow sample was not safely normalized: %#v", sample)
	}
}

func TestKnowledgeV2ReadinessThresholdsAreClamped(t *testing.T) {
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READY_MIN_SAMPLES", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READY_MAX_ERROR_PERCENT", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READY_MAX_SLOW_PERCENT", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READY_MIN_CANONICAL_HITS", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READY_MIN_HIGH_CONFIDENCE_PERCENT", "1")
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READY_FRESH_DAYS", "999999")
	thresholds := knowledgeV2ReadinessThresholdConfig()
	if thresholds.MinSamples != 1000 || thresholds.MaxErrorPercent != 25 || thresholds.MaxSlowPercent != 50 || thresholds.MinCanonicalObservations != 5000 || thresholds.MinHighConfidencePercent != 50 || thresholds.Freshness != 30*24*time.Hour {
		t.Fatalf("readiness thresholds escaped clamps: %#v", thresholds)
	}
}

func TestKnowledgeV2ShadowMetricSchemaPersistsOnlyAggregates(t *testing.T) {
	migration := strings.ToLower(strings.Join(strings.Fields(knowledgeV2ReadinessMigrationSQL), " "))
	for _, required := range []string{
		"create table if not exists codelocal_knowledge_shadow_metrics",
		"primary key(user_id,project_id)",
		"samples_total bigint",
		"error_count bigint",
		"canonical_hits_total bigint",
		"high_confidence_hits_total bigint",
		"slow_samples bigint",
		"last_sample_at bigint",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("shadow metric migration lost aggregate %q", required)
		}
	}
	for _, forbidden := range []string{"summary", "stable_key", "repository_id", "branch", "subject", "object", "predicate"} {
		if strings.Contains(migration, forbidden) || strings.Contains(strings.ToLower(upsertKnowledgeV2ShadowMetricsSQL), forbidden) {
			t.Fatalf("shadow metric persistence leaked knowledge payload field %q", forbidden)
		}
	}
}

func TestKnowledgeV2ReadinessQueriesAreTenantProjectScoped(t *testing.T) {
	for name, query := range map[string]string{
		"metrics": knowledgeV2ShadowMetricsSelectSQL,
		"health":  knowledgeV2ReadinessHealthSQL,
	} {
		normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(normalized, "user_id=$1") || !strings.Contains(normalized, "project_id=$2") {
			t.Fatalf("%s readiness query lost tenant/project scope: %s", name, normalized)
		}
	}
}
