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

func TestWalkSemanticNodesTraversesWindowsStyleNodeAndKeepsBounds(t *testing.T) {
	bounds := map[string]any{"x": 10.0, "y": 20.0, "width": 100.0, "height": 40.0}
	tree := map[string]any{
		"node": map[string]any{
			"elementId": "uia-root",
			"role":      "window",
			"children": []any{
				map[string]any{
					"elementId": "uia-save",
					"role":      "button",
					"name":      "Save",
					"enabled":   true,
					"bounds":    bounds,
				},
			},
		},
	}
	best := semanticCandidate{}
	walkSemanticNodes(tree, "save", &best)
	if best.ElementID != "uia-save" {
		t.Fatalf("expected Windows-style nested target, got %#v", best)
	}
	if best.Bounds == nil {
		t.Fatal("expected semantic target bounds to be retained")
	}
}

func TestNormalizeSemanticText(t *testing.T) {
	if got := normalizeSemanticText("  Save   Changes  "); got != "save changes" {
		t.Fatalf("unexpected normalization: %q", got)
	}
}

func TestWalkSemanticNodesMatchesVisionFallbackText(t *testing.T) {
	tree := map[string]any{"nodes": []any{
		map[string]any{"elementId": "vision:120.5:220.5", "role": "visionText", "name": "New Channel", "enabled": true,
			"bounds": map[string]any{"x": 80.0, "y": 200.0, "width": 81.0, "height": 41.0}},
	}}
	best := semanticCandidate{}
	walkSemanticNodes(tree, "New Channel", &best)
	if best.ElementID != "vision:120.5:220.5" {
		t.Fatalf("expected Vision target, got %#v", best)
	}
	if best.Score < 100 {
		t.Fatalf("expected exact Vision semantic score, got %d", best.Score)
	}
}
