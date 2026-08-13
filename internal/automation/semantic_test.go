package automation

import "testing"

func TestWalkSemanticNodesPrefersExactInteractiveTarget(t *testing.T) {
	tree := map[string]any{
		"nodes": []any{
			map[string]any{
				"elementId": "10:0",
				"role":      "AXGroup",
				"name":      "Continue to settings",
				"enabled":   true,
			},
			map[string]any{
				"elementId": "10:1",
				"role":      "AXButton",
				"name":      "Continue",
				"enabled":   true,
			},
		},
	}
	best := semanticCandidate{}
	walkSemanticNodes(tree, "continue", &best)
	if best.ElementID != "10:1" {
		t.Fatalf("expected exact interactive element, got %#v", best)
	}
	if best.Score < 100 {
		t.Fatalf("expected strong exact score, got %d", best.Score)
	}
}

func TestWalkSemanticNodesPenalizesDisabledElement(t *testing.T) {
	tree := []any{
		map[string]any{"elementId": "20:0", "role": "AXButton", "name": "Save", "enabled": false},
		map[string]any{"elementId": "20:1", "role": "AXButton", "name": "Save changes", "enabled": true},
	}
	best := semanticCandidate{}
	walkSemanticNodes(tree, "save", &best)
	if best.ElementID != "20:1" {
		t.Fatalf("expected enabled target to win, got %#v", best)
	}
}

func TestNormalizeSemanticText(t *testing.T) {
	if got := normalizeSemanticText("  Save   Changes  "); got != "save changes" {
		t.Fatalf("unexpected normalization: %q", got)
	}
}
