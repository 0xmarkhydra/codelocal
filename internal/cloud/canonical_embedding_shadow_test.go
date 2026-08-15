package cloud

import (
	"strings"
	"testing"
)

func TestCanonicalEmbeddingShadowSampleNormalizationIsBounded(t *testing.T) {
	valid, err := normalizeCanonicalEmbeddingShadowSample(CanonicalEmbeddingShadowSample{
		UserID: " user-a ", ProjectID: " project-a ", SemanticHits: 3, HighSimilarityHits: 2, DurationMillis: 99_000,
	})
	if err != nil || valid.UserID != "user-a" || valid.ProjectID != "project-a" || valid.DurationMillis != 30_000 {
		t.Fatalf("unexpected normalized embedding shadow sample: %#v err=%v", valid, err)
	}
	for _, sample := range []CanonicalEmbeddingShadowSample{
		{},
		{UserID: "user", ProjectID: "project", SemanticHits: -1},
		{UserID: "user", ProjectID: "project", SemanticHits: 1, HighSimilarityHits: 2},
	} {
		if _, err := normalizeCanonicalEmbeddingShadowSample(sample); err == nil {
			t.Fatalf("invalid embedding shadow sample accepted: %#v", sample)
		}
	}
}

func TestCanonicalEmbeddingShadowSchemaPersistsOnlyAggregates(t *testing.T) {
	lower := strings.ToLower(canonicalEmbeddingShadowMigrationSQL + "\n" + upsertCanonicalEmbeddingShadowMetricsSQL)
	for _, required := range []string{
		"codelocal_knowledge_embedding_shadow_metrics",
		"samples_total",
		"semantic_hits_total",
		"high_similarity_hits_total",
		"duration_millis_total",
		"primary key(user_id,project_id)",
		"references codelocal_projects(user_id,project_id)",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("embedding shadow schema missing %q", required)
		}
	}
	for _, forbidden := range []string{"query text", "summary", "stable_key", "repository_id", "branch text", "objective", "root_cause", "file_path", "symbol"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("embedding shadow schema unexpectedly persists content field %q", forbidden)
		}
	}
}
