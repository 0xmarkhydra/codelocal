package automation

import (
	"context"
	"errors"
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

func TestExecuteComputerSequenceDoesNotBlindRetryFailedStep(t *testing.T) {
	steps := []computerSequenceStep{
		{Operation: "click", Target: "Save"},
		{Operation: "type", Target: "Name", Text: "CodeLocal"},
	}
	calls := 0
	results, err := executeComputerSequence(context.Background(), "ax:1:0", steps, func(_ context.Context, step computerSequenceStep, _ string) (any, error) {
		calls++
		if step.Operation == "click" {
			return nil, errors.New("stale target")
		}
		return map[string]any{"ok": true}, nil
	})
	if err == nil {
		t.Fatal("failed semantic step must return a non-nil error")
	}
	if calls != 1 {
		t.Fatalf("failed semantic step must not be retried blindly, calls=%d", calls)
	}
	if len(results) != 0 {
		t.Fatalf("no later steps should execute after failure: %#v", results)
	}
	if !strings.Contains(err.Error(), "step 1") || !strings.Contains(err.Error(), "Save") {
		t.Fatalf("batch failure should preserve step context: %v", err)
	}
}

func TestComputerSequenceCriticalTargetRequiresFreshApproval(t *testing.T) {
	decision := ClassifyAutomation(Action{Domain: "computer", Operation: "run", Origin: "ax:100:0", Target: "click:Send message"})
	if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways {
		t.Fatalf("critical batch action must require fresh approval: %+v", decision)
	}
}

func TestComputerSequenceWithTypingRequiresFreshApproval(t *testing.T) {
	action := sequenceApprovalAction("ax:100:0", []computerSequenceStep{
		{Operation: "click", Target: "New Channel"},
		{Operation: "type", Target: "Channel name", Text: "CodeLocal"},
	})
	if !action.SensitiveTarget {
		t.Fatal("a dynamic semantic batch containing typing must fail closed to fresh approval")
	}
	decision := ClassifyAutomation(action)
	if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways {
		t.Fatalf("typing batch must require fresh approval: %+v", decision)
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
