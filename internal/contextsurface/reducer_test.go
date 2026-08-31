package contextsurface

import (
	"strings"
	"testing"
)

func TestReduceObservationKeepsTestFailureEvidenceWithinBudget(t *testing.T) {
	input := ToolObservation{
		Tool:           "terminal",
		Operation:      "go test ./internal/auth",
		RawArtifactRef: "artifact://tool/raw-1",
		Text: strings.Join([]string{
			"=== RUN   TestRefreshToken",
			strings.Repeat("noise line that should not dominate context ", 40),
			"--- FAIL: TestRefreshToken (0.02s)",
			"auth/service_test.go:42: expected token to rotate",
			"received stale token",
			"FAIL github.com/example/auth 0.04s",
		}, "\n"),
	}
	reduced := ReduceObservation(input, 80)
	if reduced.Kind != ObservationTests {
		t.Fatalf("kind = %s", reduced.Kind)
	}
	if !strings.Contains(reduced.Item.Text, "FAIL") || !strings.Contains(reduced.Item.Text, "service_test.go:42") {
		t.Fatalf("critical test evidence missing: %q", reduced.Item.Text)
	}
	if reduced.ReducedTokens > 80 || reduced.ReducedTokens >= reduced.OriginalTokens {
		t.Fatalf("unexpected token reduction: %#v", reduced)
	}
	if reduced.RawArtifactRef != input.RawArtifactRef || reduced.Item.Lane != LaneObservation {
		t.Fatalf("raw reference/lane lost: %#v", reduced)
	}
}

func TestReduceObservationPreservesGitStructure(t *testing.T) {
	text := strings.Join([]string{
		"diff --git a/auth.go b/auth.go",
		"index 1111111..2222222 100644",
		"--- a/auth.go",
		"+++ b/auth.go",
		"@@ -10,2 +10,3 @@ func login() {",
		"- oldCall()",
		"+ newCall()",
		"+ verifySession()",
	}, "\n")
	reduced := ReduceObservation(ToolObservation{Kind: ObservationGit, Text: text}, 100)
	for _, required := range []string{"diff --git", "@@", "+ newCall()"} {
		if !strings.Contains(reduced.Item.Text, required) {
			t.Fatalf("git structure %q missing from %q", required, reduced.Item.Text)
		}
	}
}

func TestReduceObservationFallsBackToHeadTailWithoutSemanticSignal(t *testing.T) {
	text := strings.Join([]string{
		"first informational line",
		strings.Repeat("middle informational text ", 80),
		"last informational line",
	}, "\n")
	reduced := ReduceObservation(ToolObservation{Kind: ObservationGeneric, Text: text}, 50)
	if reduced.Strategy != "head_tail_fallback" {
		t.Fatalf("strategy = %s", reduced.Strategy)
	}
	if reduced.ReducedTokens > 50 {
		t.Fatalf("fallback exceeded budget: %#v", reduced)
	}
	if reduced.Item.ID == "" || reduced.Item.Trust != "observed" {
		t.Fatalf("invalid reduced item: %#v", reduced.Item)
	}
}

func TestReduceObservationIsDeterministic(t *testing.T) {
	input := ToolObservation{Tool: "lsp", Operation: "diagnostics", Text: "main.go:12: error: undefined symbol\nmain.go:18: warning: unused value"}
	first := ReduceObservation(input, 100)
	second := ReduceObservation(input, 100)
	if first.Item.ID != second.Item.ID || first.Item.Text != second.Item.Text || first.Kind != second.Kind {
		t.Fatalf("reducer is not deterministic: first=%#v second=%#v", first, second)
	}
}
