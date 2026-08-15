package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestCanonicalEmbeddingFreshnessCardKeepsSemanticIndexDerived(t *testing.T) {
	card := canonicalEmbeddingFreshnessCard(cloud.CanonicalEmbeddingFreshnessSummary{
		Status: "degraded", ProjectCount: 4, CurrentProjects: 2, EmptyProjects: 1, StaleProjects: 1, MaxLagMS: 180_000,
	})
	for _, required := range []string{"DEGRADED", "Projects 4", "Current 2", "Empty 1", "Stale 1", "Max source lag 3m", "Canonical knowledge remains authoritative", "fail-closed"} {
		if !strings.Contains(card, required) {
			t.Fatalf("canonical embedding freshness card missing %q: %s", required, card)
		}
	}
	for _, forbidden := range []string{"summary", "stable_key", "knowledge_id", "revision_id", "query="} {
		if strings.Contains(strings.ToLower(card), forbidden) {
			t.Fatalf("canonical embedding freshness card exposed content/internal field %q: %s", forbidden, card)
		}
	}
}

func TestCanonicalEmbeddingFreshnessCardDisabledPreservesDeterministicRetrieval(t *testing.T) {
	card := canonicalEmbeddingFreshnessCard(cloud.CanonicalEmbeddingFreshnessSummary{Status: "disabled"})
	if !strings.Contains(card, "DISABLED") || !strings.Contains(card, "Deterministic canonical retrieval remains available and authoritative") {
		t.Fatalf("disabled semantic index card overclaimed dependency: %s", card)
	}
}
