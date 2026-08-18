//go:build darwin

package main

import (
	"os"
	"testing"
)

func TestParseMacControlState(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{"", macControlActive},
		{"active\n", macControlActive},
		{"PAUSED", macControlPaused},
		{"stopped", macControlStopped},
	} {
		got, err := parseMacControlState([]byte(test.raw))
		if err != nil || got != test.want {
			t.Fatalf("parseMacControlState(%q) = %q, %v; want %q", test.raw, got, err, test.want)
		}
	}
	if _, err := parseMacControlState([]byte("unknown")); err == nil {
		t.Fatal("invalid control state must fail closed")
	}
}

func TestMacOperationMutates(t *testing.T) {
	for _, operation := range []string{"focus", "semantic_click", "semantic_type", "semantic_batch", "click", "type", "key", "scroll", "drag"} {
		if !macOperationMutates(operation) {
			t.Fatalf("%s must be classified as mutating", operation)
		}
	}
	for _, operation := range []string{"status", "list_windows", "ui_tree", "screenshot", "element_read", "scene_events"} {
		if macOperationMutates(operation) {
			t.Fatalf("%s must remain observation-only", operation)
		}
	}
}

func TestMacActivityModeDistinguishesBackgroundViewingAndForeground(t *testing.T) {
	if got := macActivityMode("semantic_click", map[string]any{"windowId": "ax:1:0"}); got != "background" {
		t.Fatalf("semantic click mode = %q", got)
	}
	if got := macActivityMode("screenshot", map[string]any{"windowId": "ax:1:0"}); got != "viewing" {
		t.Fatalf("screenshot mode = %q", got)
	}
	if got := macActivityMode("click", map[string]any{"x": float64(1), "y": float64(1)}); got != "foreground" {
		t.Fatalf("coordinate click mode = %q", got)
	}
	if got := macActivityMode("click", map[string]any{"elementId": "123:0.1"}); got != "background" {
		t.Fatalf("exact element click mode = %q", got)
	}
}

func TestGuardMacComputerControlHonorsPauseAndStop(t *testing.T) {
	t.Setenv("CODELOCAL_COMPUTER_STATE_DIR", t.TempDir())
	if err := os.WriteFile(macControlStateFile(), []byte(macControlPaused+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := guardMacComputerControl("screenshot"); err != nil {
		t.Fatalf("paused control must still allow observation: %v", err)
	}
	if err := guardMacComputerControl("semantic_click"); err == nil {
		t.Fatal("paused control must block mutating input")
	}
	if err := os.WriteFile(macControlStateFile(), []byte(macControlStopped+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := guardMacComputerControl("screenshot"); err == nil {
		t.Fatal("stopped control must block screen observation")
	}
	if err := guardMacComputerControl("status"); err != nil {
		t.Fatalf("stopped control must keep status available: %v", err)
	}
}
