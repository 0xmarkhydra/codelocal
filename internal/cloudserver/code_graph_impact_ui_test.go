package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/project"
)

func TestCodeGraphImpactCardStatesBoundedEvidence(t *testing.T) {
	view := project.CodeGraphView{Impact: &project.CodeGraphImpact{
		Scope: "bounded-neighborhood", DirectCallers: 2, DirectCallees: 1, PotentialCallers: 5,
		AffectedFiles: 3, EvidenceEdges: 6, SemanticEdges: 4, AverageConfidence: .875, Risk: "medium",
	}}
	html := codeGraphImpactCard(view)
	for _, want := range []string{"Impact Analysis · MEDIUM", "2 direct callers", "5 potential upstream callers", "3 files", "4/6", "88%", "Bounded impact"} {
		if !strings.Contains(html, want) {
			t.Fatalf("impact card missing %q: %s", want, html)
		}
	}
}

func TestCodeGraphImpactCardDoesNotInventOverviewImpact(t *testing.T) {
	if html := codeGraphImpactCard(project.CodeGraphView{}); html != "" {
		t.Fatalf("overview must not render invented impact: %s", html)
	}
}
