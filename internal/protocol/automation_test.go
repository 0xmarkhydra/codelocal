package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAutomationCapabilitiesMarshalGranularFlags(t *testing.T) {
	caps := Capabilities{Automation: AutomationCapabilities{
		Browser: BrowserCapabilities{Available: true, IsolatedProfile: true, Screenshots: true, Devtools: true},
		Computer: ComputerCapabilities{Available: true, Backend: "test", WindowList: true, ScreenCapture: true, Pointer: true, Keyboard: true},
	}}
	raw, err := json.Marshal(caps)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"automation"`, `"browser"`, `"computer"`, `"windowList":true`, `"screenCapture":true`, `"pointer":true`, `"keyboard":true`} {
		if !strings.Contains(text, want) {
			t.Fatalf("capabilities JSON missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, `"secureDesktop":true`) {
		t.Fatalf("secure desktop must never be advertised as controllable: %s", text)
	}
}

func TestAutomationSideEffects(t *testing.T) {
	for _, tool := range []string{"browser_open", "browser_click", "browser_fill", "computer_click", "computer_type", "computer_drag"} {
		if !SideEffecting(tool) {
			t.Fatalf("%s must be side effecting", tool)
		}
	}
	for _, tool := range []string{"browser_status", "browser_snapshot", "browser_console", "computer_status", "computer_list_windows", "computer_ui_tree"} {
		if SideEffecting(tool) {
			t.Fatalf("%s must stay read-only", tool)
		}
	}
}
