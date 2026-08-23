package localclient

import "testing"

func TestProjectBrainTargetsExplicitDoNotIncludeRanked(t *testing.T) {
	result := map[string]any{
		"rankedFiles": []map[string]any{{"path": "unrelated.go"}},
	}
	got := projectBrainTargets(result, []string{"internal/localclient/engine.go"})
	if len(got) != 1 || got[0] != "internal/localclient/engine.go" {
		t.Fatalf("explicit targets must remain isolated, got %#v", got)
	}
}
