package cloud

import (
	"strings"
	"testing"
)

func TestCanonicalSemanticCanaryReasonBucketsAreDeterministic(t *testing.T) {
	cases := []struct {
		reason string
		claims int
		check  func(canonicalSemanticCanaryBuckets) bool
	}{
		{SemanticCanaryReasonReady, 2, func(b canonicalSemanticCanaryBuckets) bool {
			return b.readinessReady == 1 && b.applied == 1 && b.fallback == 0
		}},
		{SemanticCanaryReasonNotReady, 0, func(b canonicalSemanticCanaryBuckets) bool { return b.readinessBlocked == 1 && b.fallback == 1 }},
		{SemanticCanaryReasonReadinessError, 0, func(b canonicalSemanticCanaryBuckets) bool { return b.readinessError == 1 && b.fallback == 1 }},
		{SemanticCanaryReasonRecallError, 0, func(b canonicalSemanticCanaryBuckets) bool {
			return b.readinessReady == 1 && b.recallError == 1 && b.fallback == 1
		}},
		{SemanticCanaryReasonNoSimilarity, 0, func(b canonicalSemanticCanaryBuckets) bool {
			return b.readinessReady == 1 && b.noSimilarity == 1 && b.fallback == 1
		}},
		{SemanticCanaryReasonNoUniqueClaims, 0, func(b canonicalSemanticCanaryBuckets) bool {
			return b.readinessReady == 1 && b.noUnique == 1 && b.fallback == 1
		}},
		{SemanticCanaryReasonTimeout, 0, func(b canonicalSemanticCanaryBuckets) bool { return b.timeout == 1 && b.fallback == 1 }},
		{SemanticCanaryReasonScopeUnavailable, 0, func(b canonicalSemanticCanaryBuckets) bool { return b.scopeUnavailable == 1 && b.fallback == 1 }},
	}
	for _, tc := range cases {
		buckets, err := canonicalSemanticCanaryBucketsFor(tc.reason, tc.claims)
		if err != nil || !tc.check(buckets) {
			t.Fatalf("reason=%q claims=%d buckets=%#v err=%v", tc.reason, tc.claims, buckets, err)
		}
	}
	if _, err := canonicalSemanticCanaryBucketsFor("unknown", 0); err == nil {
		t.Fatal("unknown semantic canary reason was accepted")
	}
	if _, err := canonicalSemanticCanaryBucketsFor(SemanticCanaryReasonReady, 0); err == nil {
		t.Fatal("applied semantic canary accepted zero claims")
	}
	if _, err := canonicalSemanticCanaryBucketsFor(SemanticCanaryReasonNotReady, 1); err == nil {
		t.Fatal("fallback semantic canary accepted applied claims")
	}
}

func TestCanonicalSemanticCanarySampleNormalizationIsBounded(t *testing.T) {
	sample, buckets, err := normalizeCanonicalSemanticCanarySample(CanonicalSemanticCanarySample{
		UserID: " user-a ", ProjectID: " project-a ", Reason: SemanticCanaryReasonReady, DurationMillis: 99_000, AppliedClaims: 1,
	})
	if err != nil || sample.UserID != "user-a" || sample.ProjectID != "project-a" || sample.DurationMillis != 30_000 || buckets.applied != 1 {
		t.Fatalf("unexpected semantic canary normalization: sample=%#v buckets=%#v err=%v", sample, buckets, err)
	}
	if _, _, err := normalizeCanonicalSemanticCanarySample(CanonicalSemanticCanarySample{Reason: SemanticCanaryReasonNotReady}); err == nil {
		t.Fatal("semantic canary sample without tenant/project was accepted")
	}
}

func TestCanonicalSemanticCanarySchemaIsAggregateOnlyTenantScopedAndAdditive(t *testing.T) {
	lower := strings.ToLower(canonicalSemanticCanaryMigrationSQL + "\n" + upsertCanonicalSemanticCanaryMetricsSQL + "\n" + canonicalSemanticCanarySummarySQL)
	for _, required := range []string{
		"codelocal_knowledge_semantic_canary_metrics",
		"primary key(user_id,project_id)",
		"references codelocal_projects(user_id,project_id)",
		"attempts_total",
		"applied_count",
		"deterministic_fallback_count",
		"readiness_blocked_count",
		"readiness_error_count",
		"recall_error_count",
		"no_high_similarity_count",
		"no_unique_claim_count",
		"timeout_count",
		"slow_count",
		"where ($1='' or user_id=$1)",
		"and ($2='' or project_id=$2)",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("semantic canary aggregate contract missing %q", required)
		}
	}
	for _, forbidden := range []string{"query_text", "summary", "stable_key", "repository_id", "branch", "file_path", "symbol", "objective", "root_cause", "embedding vector", "drop table", "truncate table"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("semantic canary aggregate schema exposes or mutates forbidden field %q", forbidden)
		}
	}
}
