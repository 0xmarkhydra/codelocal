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

func TestNormalizeWorkspaceSymbolFlattensLocationAndMarksSemanticEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.go")
	symbol := map[string]any{
		"name":          "Save",
		"kind":          float64(6),
		"containerName": "*Store",
		"location": map[string]any{
			"uri": fileURI(path),
			"range": map[string]any{
				"start": map[string]any{"line": float64(11), "character": float64(4)},
				"end":   map[string]any{"line": float64(11), "character": float64(8)},
			},
		},
	}
	got := normalizeWorkspaceSymbol(symbol, "gopls")
	if got["provider"] != "gopls" || got["resolutionMode"] != "lsp" || got["confidence"] != 1.0 {
		t.Fatalf("workspace symbol semantic provenance missing: %#v", got)
	}
	if got["path"] != path || got["line"] != 12 || got["column"] != 5 || got["detail"] != "*Store" {
		t.Fatalf("workspace symbol location not normalized: %#v", got)
	}
	if got["location"] != nil {
		t.Fatalf("raw LSP location should not leak after normalization: %#v", got)
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
