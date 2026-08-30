package contextsurface

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
)

func brainPacket() projectbrain.ContextPacket {
	return projectbrain.ContextPacket{
		Fingerprint:      "brain-fingerprint",
		MutationAllowed: true,
		EffectiveRules: []projectbrain.CompiledRule{
			{ID: "required", Text: "Never expose secrets to the model.", Required: true, Lane: "mandatory", AuthorityRank: 100, Trust: "trusted"},
			{ID: "style", Text: "Prefer existing project conventions.", Lane: "relevant", AuthorityRank: 10, Trust: "trusted"},
		},
	}
}

func TestCompilePrioritizesRequiredThenActiveEvidence(t *testing.T) {
	surface := Compile(Input{
		Brain: brainPacket(),
		Active: []Item{{ID: "failure", Text: "TestRefreshToken fails in auth/service_test.go", Priority: 100}},
		Recent: []Item{{ID: "old", Text: "Earlier unrelated investigation detail", Priority: 1}},
	}, 100)
	if len(surface.Items) < 3 {
		t.Fatalf("expected required, active, and relevant context: %+v", surface)
	}
	if surface.Items[0].ID != "brain:required" || surface.Items[0].Lane != LaneBrainMandatory {
		t.Fatalf("mandatory brain rule was not first: %+v", surface.Items)
	}
	if surface.Items[1].ID != "failure" || surface.Items[1].Lane != LaneActive {
		t.Fatalf("active evidence was not prioritized: %+v", surface.Items)
	}
	if !surface.MutationAllowed || surface.Fingerprint == "" {
		t.Fatalf("unexpected safe surface state: %+v", surface)
	}
}

func TestCompileFailsClosedWhenOuterBudgetCannotHoldMandatoryRule(t *testing.T) {
	packet := brainPacket()
	packet.EffectiveRules[0].Text = "This mandatory rule is intentionally much longer than the tiny outer token budget can represent safely."
	surface := Compile(Input{Brain: packet}, 1)
	if !surface.MandatoryOverflow || surface.MutationAllowed {
		t.Fatalf("mandatory overflow must block mutation: %+v", surface)
	}
	if len(surface.OmittedItemIDs) == 0 || surface.OmittedItemIDs[0] != "brain:required" {
		t.Fatalf("missing required omission evidence: %+v", surface)
	}
}

func TestCompileDeduplicatesLaneItemsByIDAndUsesPriority(t *testing.T) {
	surface := Compile(Input{
		Brain: projectbrain.ContextPacket{Fingerprint: "brain", MutationAllowed: true},
		Active: []Item{
			{ID: "same", Text: "lower priority", Priority: 1},
			{ID: "same", Text: "duplicate id", Priority: 100},
			{ID: "high", Text: "high priority", Priority: 50},
		},
	}, 100)
	if len(surface.Items) != 2 {
		t.Fatalf("expected duplicate active id to collapse: %+v", surface.Items)
	}
	if surface.Items[0].ID != "high" || surface.Items[1].ID != "same" {
		t.Fatalf("unexpected priority order: %+v", surface.Items)
	}
}

func TestCompileDropsLowerPriorityLanesUnderPressure(t *testing.T) {
	surface := Compile(Input{
		Brain: projectbrain.ContextPacket{Fingerprint: "brain", MutationAllowed: true},
		Active: []Item{{ID: "active", Text: "active evidence"}},
		Recent: []Item{{ID: "recent", Text: "recent history that should be dropped first when budget is tight"}},
	}, 6)
	if len(surface.Items) != 1 || surface.Items[0].ID != "active" {
		t.Fatalf("expected active evidence to win tight budget: %+v", surface.Items)
	}
	if !surface.Truncated || surface.Budget.DroppedItems == 0 {
		t.Fatalf("expected truncation accounting: %+v", surface)
	}
}
