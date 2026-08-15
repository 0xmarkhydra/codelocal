package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestCanonicalGraphFreshnessCardKeepsDerivedIndexSeparateFromCanonicalTruth(t *testing.T) {
	card := canonicalGraphFreshnessCard(cloud.CanonicalGraphFreshnessSummary{
		Status: "degraded", ProjectCount: 4, CurrentProjects: 2, EmptyProjects: 1, StaleProjects: 1, MissingProjects: 0, MaxLagMS: 180_000,
	})
	for _, required := range []string{"DEGRADED", "Projects 4", "Current 2", "Empty 1", "Stale 1", "Max source lag 3m", "Canonical knowledge remains authoritative"} {
		if !strings.Contains(card, required) {
			t.Fatalf("canonical graph freshness card missing %q: %s", required, card)
		}
	}
	for _, forbidden := range []string{"summary", "stable_key", "knowledge_id", "revision_id"} {
		if strings.Contains(strings.ToLower(card), forbidden) {
			t.Fatalf("canonical graph freshness card exposed content/internal field %q: %s", forbidden, card)
		}
	}
}

func TestCanonicalGraphFreshnessCardCurrentDoesNotClaimCanonicalMutation(t *testing.T) {
	card := canonicalGraphFreshnessCard(cloud.CanonicalGraphFreshnessSummary{Status: "current", ProjectCount: 2, CurrentProjects: 1, EmptyProjects: 1})
	if !strings.Contains(card, "synchronized with canonical Knowledge V2") || strings.Contains(strings.ToLower(card), "source of truth: graph") {
		t.Fatalf("canonical graph card confused projection with canonical truth: %s", card)
	}
}
