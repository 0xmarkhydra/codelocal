package lsp

import (
	"path/filepath"
	"testing"
)

func TestNormalizeHierarchyItemAddsSemanticProvenance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	item := map[string]any{
		"name": "ResetPassword",
		"kind": float64(12),
		"uri":  fileURI(path),
		"range": map[string]any{
			"start": map[string]any{"line": float64(7), "character": float64(1)},
			"end":   map[string]any{"line": float64(10), "character": float64(1)},
		},
		"selectionRange": map[string]any{
			"start": map[string]any{"line": float64(7), "character": float64(6)},
			"end":   map[string]any{"line": float64(7), "character": float64(19)},
		},
		"data": map[string]any{"opaque": true},
	}

	got := normalizeHierarchyItem(item, "gopls")
	if got["provider"] != "gopls" || got["resolutionMode"] != "lsp" || got["confidence"] != 1.0 {
		t.Fatalf("semantic provenance missing: %#v", got)
	}
	if got["line"] != 8 || got["column"] != 7 {
		t.Fatalf("selection position not normalized: %#v", got)
	}
	if got["data"] != nil {
		t.Fatalf("opaque provider data leaked into normalized result: %#v", got)
	}
}

func TestHierarchyItemsAcceptsSingleAndArrayResults(t *testing.T) {
	single := map[string]any{"name": "A"}
	if got := hierarchyItems(single); len(got) != 1 || got[0]["name"] != "A" {
		t.Fatalf("single result=%#v", got)
	}
	array := []any{map[string]any{"name": "A"}, map[string]any{"name": "B"}}
	if got := hierarchyItems(array); len(got) != 2 || got[1]["name"] != "B" {
		t.Fatalf("array result=%#v", got)
	}
}
