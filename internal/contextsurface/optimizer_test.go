package contextsurface

import (
	"errors"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
)

func TestCompileOptimizedReducesRawObservationsAndCompactsHistory(t *testing.T) {
	input := Input{
		Brain:  projectbrain.ContextPacket{Fingerprint: "brain", MutationAllowed: true},
		Active: []Item{{ID: "active", Lane: LaneActive, Text: "Fix auth refresh regression without touching unrelated user edits.", Priority: 100}},
		Recent: []Item{
			{ID: "old-a", Lane: LaneRecent, Text: strings.Repeat("old unrelated context A ", 80)},
			{ID: "old-b", Lane: LaneRecent, Text: strings.Repeat("old unrelated context B ", 80)},
		},
	}
	raw := []ToolObservation{{
		Tool:           "terminal",
		Operation:      "go test ./internal/auth",
		RawArtifactRef: "artifact://raw-test",
		Text: strings.Join([]string{
			strings.Repeat("test runner noise ", 120),
			"--- FAIL: TestRefreshToken",
			"auth/service_test.go:42: expected rotated refresh token",
			"FAIL github.com/example/auth",
		}, "\n"),
	}}
	result, err := CompileOptimized(input, raw, OptimizationPolicy{MaxTokens: 120, TargetTokens: 80, PerObservationTokens: 40, CompactAtRatio: .7})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.RawObservationTokens <= result.Metrics.ReducedObservationTokens {
		t.Fatalf("raw observation was not reduced: %#v", result.Metrics)
	}
	if result.Metrics.FinalVisibleTokens > 120 {
		t.Fatalf("final context exceeds model budget: %#v", result.Metrics)
	}
	if result.Metrics.EstimatedAvoidedTokens <= 0 {
		t.Fatalf("missing avoided token accounting: %#v", result.Metrics)
	}
	if len(result.Reductions) != 1 || result.Reductions[0].RawArtifactRef != "artifact://raw-test" {
		t.Fatalf("raw artifact lineage lost: %#v", result.Reductions)
	}
	if !strings.Contains(result.Reductions[0].Item.Text, "TestRefreshToken") {
		t.Fatalf("semantic failure evidence lost: %q", result.Reductions[0].Item.Text)
	}
	if result.Metrics.Compacted && result.Compaction.ID == "" {
		t.Fatalf("compaction metric/descriptor mismatch: %#v", result)
	}
}

func TestCompileOptimizedRejectsImpossibleMandatoryBudget(t *testing.T) {
	input := Input{Brain: projectbrain.ContextPacket{
		Fingerprint:     "brain",
		MutationAllowed: true,
		EffectiveRules: []projectbrain.CompiledRule{{
			ID:            "required",
			Text:          strings.Repeat("mandatory rule must remain visible ", 100),
			Required:      true,
			Lane:          "mandatory",
			AuthorityRank: 100,
			Trust:         "trusted",
		}},
	}}
	_, err := CompileOptimized(input, nil, OptimizationPolicy{MaxTokens: 10, TargetTokens: 8})
	if !errors.Is(err, ErrContextOptimizationInvalid) {
		t.Fatalf("expected invalid optimization, got %v", err)
	}
}

func TestCompileOptimizedSmallSurfaceAvoidsCompaction(t *testing.T) {
	input := Input{Brain: projectbrain.ContextPacket{Fingerprint: "brain", MutationAllowed: true}, Active: []Item{{ID: "active", Text: "small context"}}}
	result, err := CompileOptimized(input, nil, OptimizationPolicy{MaxTokens: 200, TargetTokens: 150})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.Compacted || result.Compaction.ID != "" {
		t.Fatalf("small surface should not compact: %#v", result)
	}
	if result.Surface.Budget.MaxTokens != 200 {
		t.Fatalf("final budget = %#v", result.Surface.Budget)
	}
}
