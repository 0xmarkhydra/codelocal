package contextsurface

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCompactPreservesRequiredEvidenceAndIsReversible(t *testing.T) {
	surface := Surface{
		Version:          Version,
		Fingerprint:      "surface-before",
		BrainFingerprint: "brain",
		Items: []Item{
			{ID: "required", Lane: LaneBrainMandatory, Text: "Never expose secrets to the model.", Required: true, Priority: 100, Trust: "trusted"},
			{ID: "active", Lane: LaneActive, Text: "Current auth failure is in refreshSession and must preserve user edits.", Priority: 100, Trust: "observed"},
			{ID: "observation", Lane: LaneObservation, Text: "auth/service_test.go:42 reports stale refresh token after rotation.", Priority: 80, Trust: "observed"},
			{ID: "old-a", Lane: LaneRecent, Text: strings.Repeat("older investigation detail A ", 80), Priority: 1},
			{ID: "old-b", Lane: LaneRecent, Text: strings.Repeat("older investigation detail B ", 80), Priority: 1},
		},
		Budget:          Budget{MaxTokens: 1000},
		MutationAllowed: true,
	}
	compacted, record, err := Compact(surface, 80)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || len(record.HiddenItemIDs) == 0 || record.MarkerItemID == "" {
		t.Fatalf("missing compaction descriptor: %#v", record)
	}
	if compacted.Budget.EstimatedTokens > 80 {
		t.Fatalf("compacted surface exceeds target: %#v", compacted.Budget)
	}
	if !hasContextItem(compacted.Items, "required") {
		t.Fatalf("required evidence was hidden: %#v", compacted.Items)
	}
	if !hasContextItem(compacted.Items, record.MarkerItemID) {
		t.Fatalf("checkpoint marker missing: %#v", compacted.Items)
	}
	if !compacted.Truncated || compacted.Fingerprint == surface.Fingerprint {
		t.Fatalf("compaction accounting/fingerprint not updated: %#v", compacted)
	}

	expanded := Expand(record, []string{"old-a"})
	if len(expanded) != 1 || expanded[0].ID != "old-a" || !strings.Contains(expanded[0].Text, "older investigation detail A") {
		t.Fatalf("expand failed: %#v", expanded)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "older investigation detail A") || strings.Contains(string(raw), "older investigation detail B") {
		t.Fatalf("durable descriptor serialized hidden raw text: %s", raw)
	}
}

func TestCompactFailsClosedWhenMandatoryEvidenceCannotFit(t *testing.T) {
	surface := Surface{
		Fingerprint: "surface",
		Items:       []Item{{ID: "required", Lane: LaneBrainMandatory, Text: strings.Repeat("mandatory rule ", 50), Required: true}},
	}
	if _, _, err := Compact(surface, 1); !errors.Is(err, ErrCompactionMandatoryOverflow) {
		t.Fatalf("expected mandatory overflow, got %v", err)
	}
}

func TestCompactNoopWhenAlreadyWithinTarget(t *testing.T) {
	surface := Surface{
		Fingerprint: "same",
		Items:       []Item{{ID: "active", Lane: LaneActive, Text: "small context"}},
	}
	compacted, record, err := Compact(surface, 100)
	if err != nil {
		t.Fatal(err)
	}
	if compacted.Fingerprint != surface.Fingerprint || record.ID != "" || len(record.HiddenItemIDs) != 0 {
		t.Fatalf("expected no-op compaction: compacted=%#v record=%#v", compacted, record)
	}
}

func hasContextItem(items []Item, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
