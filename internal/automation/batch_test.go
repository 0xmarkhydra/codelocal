package automation

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestParseComputerSequenceAcceptsBoundedBackgroundSemanticSteps(t *testing.T) {
	windowID, steps, err := parseComputerSequence(map[string]any{
		"windowId": "ax:100:0",
		"steps": []any{
			map[string]any{"action": "click", "target": "New Channel"},
			map[string]any{"action": "type", "target": "Channel name", "text": "CodeLocal Updates"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if windowID != "ax:100:0" || len(steps) != 2 || steps[0].Operation != "click" || steps[1].Text != "CodeLocal Updates" {
		t.Fatalf("unexpected parsed sequence: window=%q steps=%+v", windowID, steps)
	}
}

func TestParseComputerSequenceRejectsPhysicalOrCrossWindowSteps(t *testing.T) {
	cases := []map[string]any{
		{"windowId": "screen:main", "steps": []any{map[string]any{"action": "click", "target": "Save"}}},
		{"windowId": "ax:1:0", "steps": []any{map[string]any{"action": "scroll", "target": "List"}}},
		{"windowId": "ax:1:0", "steps": []any{map[string]any{"action": "click", "windowId": "ax:2:0", "target": "Save"}}},
		{"windowId": "ax:1:0", "steps": []any{map[string]any{"action": "click", "x": 20, "y": 20}}},
	}
	for _, args := range cases {
		if _, _, err := parseComputerSequence(args); err == nil {
			t.Fatalf("unsafe sequence accepted: %+v", args)
		}
	}
}

func TestComputerSequenceCriticalTargetRequiresFreshApproval(t *testing.T) {
	decision := ClassifyAutomation(Action{Domain: "computer", Operation: "run", Origin: "ax:100:0", Target: "click:Send message"})
	if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways {
		t.Fatalf("critical batch action must require fresh approval: %+v", decision)
	}
}

func TestAutomationApprovalFingerprintBindsTextWithoutExposingIt(t *testing.T) {
	first := automationCommand(Action{Domain: "computer", Operation: "type", Origin: "ax:1:0", Target: "Field", Text: "super-secret-one"})
	second := automationCommand(Action{Domain: "computer", Operation: "type", Origin: "ax:1:0", Target: "Field", Text: "super-secret-two"})
	if first == second {
		t.Fatal("different typed values must produce different approval fingerprints")
	}
	if strings.Contains(first, "super-secret-one") || strings.Contains(second, "super-secret-two") {
		t.Fatal("approval command must not expose typed text")
	}
	if !strings.Contains(first, "text-sha256:") {
		t.Fatal("approval command should include a non-secret text digest")
	}
}
