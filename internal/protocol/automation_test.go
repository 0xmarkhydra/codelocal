package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAutomationCapabilitiesMarshalGranularFlags(t *testing.T) {
	caps := Capabilities{Automation: AutomationCapabilities{
		Browser: BrowserCapabilities{Available: true, IsolatedProfile: true, Screenshots: true, Devtools: true},
		Computer: ComputerCapabilities{
			Available: true, DesktopAvailable: true, Backend: "test", PersistentEngine: true, SceneCache: true, BatchActions: true, WindowList: true, ScreenCapture: true, Pointer: true, Keyboard: true,
			Mobile: &MobileCapabilities{Available: true, Backend: "mobile-mcp", Version: "1.0.2", Managed: true, IOS: true, Android: true, DeviceList: true, Pointer: true, Keyboard: true, AppLifecycle: true},
		},
	}}
	raw, err := json.Marshal(caps)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"automation"`, `"browser"`, `"computer"`, `"desktopAvailable":true`, `"persistentEngine":true`, `"sceneCache":true`, `"batchActions":true`, `"windowList":true`, `"screenCapture":true`, `"pointer":true`, `"keyboard":true`, `"mobile"`, `"backend":"mobile-mcp"`, `"version":"1.0.2"`, `"managed":true`, `"ios":true`, `"android":true`} {
		if !strings.Contains(text, want) {
			t.Fatalf("capabilities JSON missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, `"secureDesktop":true`) {
		t.Fatalf("secure desktop must never be advertised as controllable: %s", text)
	}
}

func TestAutomationSideEffects(t *testing.T) {
	for _, tool := range []string{"browser_open", "browser_click", "browser_fill", "computer_click", "computer_type", "computer_drag", "computer_run", "computer_launch_app", "computer_install_app", "computer_uninstall_app", "computer_open_url", "computer_set_orientation", "computer_record_start", "computer_record_stop"} {
		if !SideEffecting(tool) {
			t.Fatalf("%s must be side effecting", tool)
		}
	}
	for _, tool := range []string{"browser_status", "browser_snapshot", "browser_console", "computer_status", "computer_list_windows", "computer_ui_tree", "computer_list_devices", "computer_list_apps", "computer_get_orientation", "computer_list_crashes", "computer_get_crash"} {
		if SideEffecting(tool) {
			t.Fatalf("%s must stay read-only", tool)
		}
	}
}
