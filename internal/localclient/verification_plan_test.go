package localclient

import "testing"

func TestRecommendedChecksForGoChangesAreTargeted(t *testing.T) {
	project := map[string]any{
		"languages":    []string{"go"},
		"testCommands": []string{"go test ./..."},
	}
	checks := recommendedChecksForChanges(project, []string{
		"internal/orchestration/planner.go",
		"internal/orchestration/progress.go",
	})
	want := map[string]bool{
		"git diff --check":                 false,
		"go test ./internal/orchestration": false,
		"go vet ./...":                     false,
	}
	for _, check := range checks {
		if _, ok := want[check]; ok {
			want[check] = true
		}
	}
	for check, found := range want {
		if !found {
			t.Fatalf("missing targeted verification %q in %#v", check, checks)
		}
	}
}

func TestRecommendedChecksForManifestChangeWidenScope(t *testing.T) {
	project := map[string]any{
		"languages":         []string{"typescript"},
		"typecheckCommands": []string{"npm run typecheck"},
		"testCommands":      []string{"npm run test"},
		"buildCommands":     []string{"npm run build"},
	}
	checks := recommendedChecksForChanges(project, []string{"package.json", "src/index.ts"})
	found := map[string]bool{}
	for _, check := range checks {
		found[check] = true
	}
	for _, want := range []string{"git diff --check", "npm run typecheck", "npm run test", "npm run build"} {
		if !found[want] {
			t.Fatalf("manifest verification missing %q: %#v", want, checks)
		}
	}
}

func TestRecommendedChecksForDocsAvoidExpensiveSuites(t *testing.T) {
	project := map[string]any{
		"testCommands":  []string{"go test ./..."},
		"buildCommands": []string{"go build ./..."},
	}
	checks := recommendedChecksForChanges(project, []string{"README.md", "ROADMAP.md"})
	if len(checks) != 1 || checks[0] != "git diff --check" {
		t.Fatalf("docs-only verification should stay cheap: %#v", checks)
	}
}
