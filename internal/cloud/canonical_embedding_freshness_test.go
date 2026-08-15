package cloud

import (
	"strings"
	"testing"
)

func TestCanonicalEmbeddingFreshnessClassifiesDerivedState(t *testing.T) {
	now := int64(10_000)
	cases := []struct {
		name string
		in   CanonicalEmbeddingFreshness
		want string
	}{
		{name: "empty", in: CanonicalEmbeddingFreshness{}, want: "empty"},
		{name: "missing", in: CanonicalEmbeddingFreshness{SourceRevisionCount: 2, SourceUpdatedAt: 8_000, ProjectedSourceCount: -1, ProjectedRevisionCount: -1}, want: "missing"},
		{name: "stale source", in: CanonicalEmbeddingFreshness{SourceRevisionCount: 2, SourceUpdatedAt: 9_000, ProjectedSourceCount: 1, ProjectedSourceUpdatedAt: 8_000, ProjectedRevisionCount: 1, VectorCount: 1, ProjectedAt: 8_500}, want: "stale"},
		{name: "stale vector count", in: CanonicalEmbeddingFreshness{SourceRevisionCount: 2, SourceUpdatedAt: 9_000, ProjectedSourceCount: 2, ProjectedSourceUpdatedAt: 9_000, ProjectedRevisionCount: 2, VectorCount: 1, ProjectedAt: 9_500}, want: "stale"},
		{name: "current with unsafe skip", in: CanonicalEmbeddingFreshness{SourceRevisionCount: 3, SourceUpdatedAt: 9_000, ProjectedSourceCount: 3, ProjectedSourceUpdatedAt: 9_000, ProjectedRevisionCount: 2, VectorCount: 2, ProjectedAt: 9_500}, want: "current"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCanonicalEmbeddingFreshness(tc.in, now)
			if got.Status != tc.want {
				t.Fatalf("status=%q want=%q value=%#v", got.Status, tc.want, got)
			}
		})
	}
}

func TestCanonicalEmbeddingFreshnessSummaryDoesNotDegradeEmptyProjects(t *testing.T) {
	clean := summarizeCanonicalEmbeddingFreshness([]CanonicalEmbeddingFreshness{{Status: "current"}, {Status: "empty"}})
	if clean.Status != "current" || clean.CurrentProjects != 1 || clean.EmptyProjects != 1 {
		t.Fatalf("unexpected clean embedding freshness: %#v", clean)
	}
	degraded := summarizeCanonicalEmbeddingFreshness([]CanonicalEmbeddingFreshness{{Status: "missing", LagMS: 10}, {Status: "stale", LagMS: 20}})
	if degraded.Status != "degraded" || degraded.MissingProjects != 1 || degraded.StaleProjects != 1 || degraded.MaxLagMS != 20 {
		t.Fatalf("unexpected degraded embedding freshness: %#v", degraded)
	}
}

func TestCanonicalEmbeddingFreshnessQueryIsAggregateOnlyAndTenantScoped(t *testing.T) {
	lower := strings.ToLower(canonicalEmbeddingFreshnessSQL)
	for _, required := range []string{
		"privacy_classification='private_project'",
		"o.status='active'",
		"where ($1='' or p.user_id=$1)",
		"and ($5='' or p.project_id=$5)",
		"provider=$2",
		"model=$3",
		"model_version=$4",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("freshness query missing %q: %s", required, lower)
		}
	}
	for _, forbidden := range []string{"r.summary", "o.stable_key", "r.subject", "r.object", "objective", "root_cause"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("freshness query leaks content field %q", forbidden)
		}
	}
}
