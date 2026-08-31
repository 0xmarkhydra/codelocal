package automation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileMCPEnvForcesSafeManagedDefaults(t *testing.T) {
	t.Setenv("MOBILEMCP_DISABLE_TELEMETRY", "0")
	t.Setenv("MOBILEMCP_ALLOW_UNSAFE_URLS", "1")
	t.Setenv("MOBILEMCP_AUTH", "should-not-leak")

	env := mobileMCPEnv()
	joined := "\n" + strings.Join(env, "\n") + "\n"
	if !strings.Contains(joined, "\nMOBILEMCP_DISABLE_TELEMETRY=1\n") {
		t.Fatalf("managed backend must disable telemetry: %q", joined)
	}
	if !strings.Contains(joined, "\nMOBILEMCP_ALLOW_UNSAFE_URLS=0\n") {
		t.Fatalf("managed backend must keep unsafe URL schemes disabled: %q", joined)
	}
	if strings.Contains(joined, "MOBILEMCP_AUTH=") {
		t.Fatalf("host Mobile MCP auth secret must not leak into managed stdio backend: %q", joined)
	}
}

func TestSafeMobileWorkspacePathStaysInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	inside, err := safeMobileWorkspacePath(root, filepath.Join("artifacts", "app.apk"))
	if err != nil {
		t.Fatalf("workspace-relative path rejected: %v", err)
	}
	if want := filepath.Join(root, "artifacts", "app.apk"); inside != want {
		t.Fatalf("safe path=%q want %q", inside, want)
	}
	if _, err := safeMobileWorkspacePath(root, filepath.Join("..", "escape.apk")); err == nil {
		t.Fatal("path traversal outside workspace must be rejected")
	}
	outside := filepath.Join(filepath.Dir(root), "outside.mp4")
	if _, err := safeMobileWorkspacePath(root, outside); err == nil {
		t.Fatal("absolute path outside workspace must be rejected")
	}
}

func TestMobileSemanticElementResolutionHelpers(t *testing.T) {
	element := map[string]any{
		"label": "Continue",
		"type":  "Button",
		"coordinates": map[string]any{
			"x": 10.0, "y": 20.0, "width": 100.0, "height": 40.0,
		},
	}
	if score := mobileElementScore(element, "continue"); score < 100 {
		t.Fatalf("exact semantic label should be preferred, score=%d", score)
	}
	x, y, ok := mobileElementCenter(element)
	if !ok || x != 60 || y != 40 {
		t.Fatalf("element center=(%v,%v,%v), want (60,40,true)", x, y, ok)
	}
}

func TestMobileDirectionCompatibility(t *testing.T) {
	direction, distance, err := mobileDirection(map[string]any{"deltaY": 240}, false)
	if err != nil || direction != "up" || distance != 240 {
		t.Fatalf("desktop-style scroll delta should map to mobile swipe: direction=%q distance=%v err=%v", direction, distance, err)
	}
	direction, distance, err = mobileDirection(map[string]any{"fromX": 200, "fromY": 100, "toX": 20, "toY": 100}, true)
	if err != nil || direction != "left" || distance != 180 {
		t.Fatalf("desktop-style drag should map to mobile swipe: direction=%q distance=%v err=%v", direction, distance, err)
	}
}

func TestMobileComputerRoutingKeepsStatusGlobal(t *testing.T) {
	if isMobileComputerRequest("computer_status", map[string]any{"device": "iphone"}) {
		t.Fatal("computer_status must stay a global desktop+mobile status operation")
	}
	if !isMobileComputerRequest("computer_click", map[string]any{"device": "iphone"}) {
		t.Fatal("computer click with device must route to managed mobile backend")
	}
	if !isMobileComputerRequest("computer_list_devices", nil) {
		t.Fatal("device discovery must route to managed mobile backend")
	}
	if isMobileComputerRequest("computer_click", map[string]any{}) {
		t.Fatal("desktop click without device must stay on native desktop backend")
	}
}

func TestMobileMCPCommandUsesPackagedPinnedEntry(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "node_modules", "@mobilenext", "mobile-mcp", "lib", "index.js")
	if err := os.MkdirAll(filepath.Dir(entry), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("// stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_MOBILE_MCP_CLI", "")
	t.Setenv("CODELOCAL_PACKAGE_ROOT", root)
	command, args, ok := mobileMCPCommand()
	if !ok || command == "" || len(args) != 1 || args[0] != entry {
		t.Fatalf("packaged Mobile MCP command not resolved: command=%q args=%#v ok=%v", command, args, ok)
	}
}
