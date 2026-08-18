package project

import "testing"

func TestArchitectureClusterNamesPreferDomainRegions(t *testing.T) {
	snapshot := &CodeGraphSnapshot{RepositoryPath: "."}
	cases := map[string]string{
		"internal/cloud/store.go":        "internal/cloud",
		"internal/project/project.go":    "internal/project",
		"cmd/codelocal/main.go":          "cmd/codelocal",
		"src/client-v2.ts":               "src",
		"README.md":                      "(root)",
		"packages/api/src/handler.ts":    "packages/api",
		"services/auth/token/service.go": "services/auth",
	}
	for path, want := range cases {
		if got := architectureClusterName(path, snapshot); got != want {
			t.Fatalf("cluster %q=%q want %q", path, got, want)
		}
	}
}

func TestArchitectureGraphAggregatesImportTraffic(t *testing.T) {
	snapshot := &CodeGraphSnapshot{RepositoryID: "repo", RepositoryPath: ".", Revision: "rev"}
	raw := []map[string]any{
		{"from": "internal/cloud/a.go", "to": "internal/project/a.go", "specifier": "../project", "fromRepositoryId": "repo", "fromRepositoryPath": ".", "toRepositoryId": "repo", "toRepositoryPath": "."},
		{"from": "internal/cloud/b.go", "to": "internal/project/b.go", "specifier": "../project", "fromRepositoryId": "repo", "fromRepositoryPath": ".", "toRepositoryId": "repo", "toRepositoryPath": "."},
		{"from": "internal/cloud/a.go", "to": "github.com/jackc/pgx/v5", "specifier": "github.com/jackc/pgx/v5", "fromRepositoryId": "repo", "fromRepositoryPath": "."},
	}
	nodes, edges, truncated := architectureGraph("rev", raw, snapshot, 20)
	if truncated {
		t.Fatal("small architecture graph should not be truncated")
	}
	if len(nodes) != 3 {
		t.Fatalf("nodes=%d want 3: %#v", len(nodes), nodes)
	}
	foundAggregate := false
	for _, edge := range edges {
		if edge.Count == 2 {
			foundAggregate = true
			if edge.Relation != "IMPORTS" || edge.ResolutionMode != "structural" {
				t.Fatalf("unexpected aggregated edge: %#v", edge)
			}
		}
	}
	if !foundAggregate {
		t.Fatalf("expected aggregated import count: %#v", edges)
	}
}

func TestArchitectureGraphNeverExceedsNodeBound(t *testing.T) {
	snapshot := &CodeGraphSnapshot{RepositoryID: "repo", RepositoryPath: ".", Revision: "rev"}
	raw := []map[string]any{
		{"from": "internal/a/a.go", "to": "internal/b/b.go", "specifier": "../b", "fromRepositoryId": "repo", "fromRepositoryPath": ".", "toRepositoryId": "repo", "toRepositoryPath": "."},
		{"from": "internal/c/c.go", "to": "internal/d/d.go", "specifier": "../d", "fromRepositoryId": "repo", "fromRepositoryPath": ".", "toRepositoryId": "repo", "toRepositoryPath": "."},
	}
	nodes, _, truncated := architectureGraph("rev", raw, snapshot, 3)
	if len(nodes) > 3 {
		t.Fatalf("architecture graph exceeded node cap: %d", len(nodes))
	}
	if !truncated {
		t.Fatal("graph should report truncation when another edge would exceed the node cap")
	}
}

func TestCodeGraphViewModeDefaultsToArchitecture(t *testing.T) {
	for input, want := range map[string]string{"": "architecture", "architecture": "architecture", "unknown": "architecture", "files": "files", "FILES": "files"} {
		if got := codeGraphViewMode(input); got != want {
			t.Fatalf("view %q=%q want %q", input, got, want)
		}
	}
}
