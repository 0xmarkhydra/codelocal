package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestCanonicalSemanticCanaryCardShowsAggregateOutcomesOnly(t *testing.T) {
	card := canonicalSemanticCanaryCard(cloud.CanonicalSemanticCanaryMetrics{
		AttemptsTotal: 100, AppliedCount: 34, AppliedClaimsTotal: 50, DeterministicFallbackCount: 66,
		ReadinessBlockedCount: 20, ReadinessErrorCount: 2, RecallErrorCount: 3, TimeoutCount: 4, SlowCount: 6,
	})
	for _, required := range []string{"OBSERVING", "Attempts 100", "Applied 34", "Claims 50", "Deterministic fallback 66", "Readiness blocked 20", "Errors 5", "Timeout 4", "Slow 6", "Aggregate-only"} {
		if !strings.Contains(card, required) {
			t.Fatalf("semantic canary card missing %q: %s", required, card)
		}
	}
	lower := strings.ToLower(card)
	for _, forbidden := range []string{"query=", "summary", "stable_key", "repository_id", "file_path", "symbol=", "objective", "root_cause"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("semantic canary card exposed forbidden field %q: %s", forbidden, card)
		}
	}
}

func TestCanonicalSemanticCanaryCardCollectsBeforeFirstSample(t *testing.T) {
	card := canonicalSemanticCanaryCard(cloud.CanonicalSemanticCanaryMetrics{})
	if !strings.Contains(card, "COLLECTING") || !strings.Contains(card, "Deterministic Hybrid remains the fallback") {
		t.Fatalf("empty semantic canary card overclaimed rollout health: %s", card)
	}
}
