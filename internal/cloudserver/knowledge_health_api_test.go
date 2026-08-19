package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestKnowledgeHealthDTOsExposeOnlyAggregateHealth(t *testing.T) {
	pipeline := knowledgePipelineHealth(cloud.DurableOutboxHealth{
		Status: "degraded", PendingCount: 3, ProcessingCount: 2, RetryingCount: 1,
		DeadCount: 4, ProcessedLastHour: 9, OldestActiveAgeMS: 1200, MaxAttempts: 99,
	}, true)
	if !pipeline.Available || pipeline.Status != "degraded" || pipeline.DeadCount != 4 {
		t.Fatalf("unexpected pipeline DTO: %#v", pipeline)
	}
	raw, err := json.Marshal(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "maxAttempts") {
		t.Fatalf("internal retry-policy detail leaked into health DTO: %s", raw)
	}

	semantic := knowledgeSemanticIndexHealth(cloud.CanonicalEmbeddingFreshnessSummary{
		Status: "degraded", ProjectCount: 4, CurrentProjects: 2, EmptyProjects: 1,
		StaleProjects: 1, MissingProjects: 0, MaxLagMS: 42, ConfigError: "private provider config detail",
	}, true)
	raw, err = json.Marshal(semantic)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private provider config detail") || strings.Contains(string(raw), "configError") {
		t.Fatalf("semantic config detail leaked into health DTO: %s", raw)
	}
}

func TestCollectiveRecommendationResourcesDropPatternIdentifiers(t *testing.T) {
	items := collectiveRecommendationResources([]cloud.CollectiveRecommendation{{
		PatternKey: "pattern_private_internal_identifier",
		Fingerprint: cloud.CollectiveFingerprint{
			TaskKind: "refactor", CheckProfile: []string{"test", "lint"}, FileCountBucket: "2-3",
			SymbolBucket: "4-8", ExecutionTool: "edit", QualityBucket: "95-100", SkillUsed: true, DiffObserved: true,
		},
		ContributorCount: 22, SampleCount: 40, MeanUserSuccessRate: 1.7, LastSeenAt: 1234,
	}})
	if len(items) != 1 || items[0].MeanUserSuccessRate != 1 || items[0].ContributorCount != 22 {
		t.Fatalf("unexpected collective recommendation DTO: %#v", items)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	if strings.Contains(serialized, "pattern_private_internal_identifier") || strings.Contains(serialized, "patternKey") {
		t.Fatalf("collective pattern identifier leaked: %s", serialized)
	}
}

func TestKnowledgeCanaryHealthUsesAggregateErrorCount(t *testing.T) {
	value := knowledgeCanaryHealth(cloud.CanonicalSemanticCanaryMetrics{
		AttemptsTotal: 10, AppliedCount: 4, ReadinessErrorCount: 2, RecallErrorCount: 3, TimeoutCount: 1,
	}, true)
	if !value.Available || value.ErrorCount != 5 || value.AttemptsTotal != 10 || value.TimeoutCount != 1 {
		t.Fatalf("unexpected canary DTO: %#v", value)
	}
}
