package automation

import "testing"

func TestComputerVerificationModeDefaultsSemanticActionsToTarget(t *testing.T) {
	args := map[string]any{"verify": true}
	if got := computerVerificationMode(args, true); got != computerVerifyTarget {
		t.Fatalf("semantic verify mode = %q, want target", got)
	}
	if got := computerVerificationMode(args, false); got != computerVerifyScene {
		t.Fatalf("non-targetable verify mode = %q, want scene", got)
	}
}

func TestComputerVerificationModeHonorsSceneOverride(t *testing.T) {
	args := map[string]any{"verify": true, "verifyMode": "scene"}
	if got := computerVerificationMode(args, true); got != computerVerifyScene {
		t.Fatalf("verify mode = %q, want scene", got)
	}
}

func TestTypeVerificationDoesNotEchoTypedValue(t *testing.T) {
	snapshot := verificationElementSnapshot("type", "super-secret", map[string]any{
		"elementId": "10:0.2",
		"role":      "AXTextField",
		"value":     "super-secret",
		"enabled":   true,
	})
	if _, exists := snapshot["value"]; exists {
		t.Fatal("type verification must not echo the current field value")
	}
	if matches, _ := snapshot["valueMatches"].(bool); !matches {
		t.Fatalf("expected typed value match, got %#v", snapshot)
	}
	if length, _ := snapshot["valueLength"].(int); length != len([]rune("super-secret")) {
		t.Fatalf("unexpected typed value length: %#v", snapshot)
	}
}

func TestResolvedElementIDUsesSemanticActionResult(t *testing.T) {
	result := map[string]any{"resolvedTarget": map[string]any{"elementId": "42:0.1"}}
	if got := resolvedElementID(result); got != "42:0.1" {
		t.Fatalf("resolved element id = %q", got)
	}
}
