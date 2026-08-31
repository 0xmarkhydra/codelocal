package mcpgateway

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAutomationCapabilityGateUsesNestedProtocolV3Capabilities(t *testing.T) {
	workspace := &gateway.WorkspaceView{ProtocolVersion: 3, Capabilities: map[string]any{}}
	if err := ensureAutomationOperationSupported("browser_status", workspace); err != nil {
		t.Fatalf("status should explain an unavailable capability: %v", err)
	}
	if err := ensureAutomationOperationSupported("browser_open", workspace); err == nil {
		t.Fatal("browser_open must be rejected until the client advertises availability")
	}

	workspace.Capabilities["automation"] = map[string]any{
		"browser": map[string]any{"available": true},
		"computer": map[string]any{
			"available": true, "windowList": true, "screenCapture": true,
			"uiTree": true, "pointer": false, "keyboard": true,
		},
	}
	if err := ensureAutomationOperationSupported("browser_open", workspace); err != nil {
		t.Fatalf("advertised browser capability should pass: %v", err)
	}
	if err := ensureAutomationOperationSupported("computer_observe", workspace); err != nil {
		t.Fatalf("computer_observe should require only window metadata: %v", err)
	}
	if err := ensureAutomationOperationSupported("computer_click", workspace); err == nil || !strings.Contains(err.Error(), "pointer") {
		t.Fatalf("computer_click should honor granular pointer capability: %v", err)
	}
	computerCaps := workspace.Capabilities["automation"].(map[string]any)["computer"].(map[string]any)
	computerCaps["pointer"] = true
	if err := ensureAutomationOperationSupported("computer_click", workspace); err != nil {
		t.Fatalf("advertised pointer capability should pass: %v", err)
	}
	if err := ensureAutomationOperationSupported("computer_run", workspace); err == nil || !strings.Contains(err.Error(), "batchActions") {
		t.Fatalf("computer_run must require explicit batchActions capability: %v", err)
	}
	computerCaps["batchActions"] = true
	if err := ensureAutomationOperationSupported("computer_run", workspace); err != nil {
		t.Fatalf("advertised batchActions capability should pass: %v", err)
	}
}

func TestAutomationCapabilityGateRoutesDesktopAndMobileIndependently(t *testing.T) {
	workspace := &gateway.WorkspaceView{ProtocolVersion: 3, Capabilities: map[string]any{
		"automation": map[string]any{
			"computer": map[string]any{
				"available":        true,
				"desktopAvailable": false,
				"mobile": map[string]any{
					"available": true, "deviceList": true, "uiTree": true, "screenCapture": true,
					"pointer": true, "keyboard": true, "appLifecycle": true, "openURL": true,
					"orientation": true, "recording": true, "crashReports": true,
				},
			},
		},
	}}
	if err := ensureAutomationOperationSupported("computer_list_devices", workspace); err != nil {
		t.Fatalf("managed mobile device discovery should pass: %v", err)
	}
	if err := ensureAutomationOperationSupported("computer_click", workspace, map[string]any{"device": "iphone"}); err != nil {
		t.Fatalf("mobile click should use nested mobile pointer capability: %v", err)
	}
	if err := ensureAutomationOperationSupported("computer_click", workspace); err == nil || !strings.Contains(err.Error(), "desktop") {
		t.Fatalf("mobile-only client must not accidentally satisfy desktop click: %v", err)
	}
	mobile := workspace.Capabilities["automation"].(map[string]any)["computer"].(map[string]any)["mobile"].(map[string]any)
	mobile["pointer"] = false
	if err := ensureAutomationOperationSupported("computer_click", workspace, map[string]any{"device": "iphone"}); err == nil || !strings.Contains(err.Error(), "pointer") {
		t.Fatalf("mobile click must honor granular mobile pointer capability: %v", err)
	}
}

func TestCompactComputerRoutesMobileWithoutAddingTopLevelTool(t *testing.T) {
	var computer compactToolDef
	for _, tool := range compactAutomationToolDefinitions() {
		if tool.Name == "computer" {
			computer = tool
			break
		}
	}
	if computer.Resolve == nil {
		t.Fatal("computer compact tool not registered")
	}
	devices, _, err := computer.Resolve(map[string]any{"action": "devices"})
	if err != nil || devices.RuntimeTool != "computer_list_devices" {
		t.Fatalf("mobile device discovery did not resolve: %#v %v", devices, err)
	}
	click, forwarded, err := computer.Resolve(map[string]any{"action": "click", "device": "iphone", "target": "Continue"})
	if err != nil || click.RuntimeTool != "computer_click" {
		t.Fatalf("semantic mobile click did not resolve: %#v %v", click, err)
	}
	if forwarded["device"] != "iphone" || forwarded["target"] != "Continue" {
		t.Fatalf("semantic mobile arguments were not forwarded: %#v", forwarded)
	}
	if _, _, err := computer.Resolve(map[string]any{"action": "run", "device": "iphone", "windowId": "ignored", "steps": []any{map[string]any{"action": "click", "target": "Continue"}}}); err == nil {
		t.Fatal("mobile run batching must stay blocked so each device action remains approval-scoped")
	}
}

func TestCompactBrowserForwardsVerify(t *testing.T) {
	var browser compactToolDef
	for _, tool := range compactAutomationToolDefinitions() {
		if tool.Name == "browser" {
			browser = tool
			break
		}
	}
	if browser.Resolve == nil {
		t.Fatal("browser compact tool not registered")
	}
	operation, forwarded, err := browser.Resolve(map[string]any{"action": "click", "ref": "e12", "verify": true})
	if err != nil || operation.RuntimeTool != "browser_click" {
		t.Fatalf("browser click did not resolve: %#v %v", operation, err)
	}
	if forwarded["verify"] != true {
		t.Fatalf("browser verify was not forwarded: %#v", forwarded)
	}
}

func TestCompactAutomationRegistryStableAcrossBrowserApprovalReplay(t *testing.T) {
	for iteration := 0; iteration < 3; iteration++ {
		var browserFound, computerFound bool
		for _, tool := range compactAutomationToolDefinitions() {
			switch tool.Name {
			case "browser":
				browserFound = true
				operation, forwarded, err := tool.Resolve(map[string]any{
					"action":        "open",
					"url":           "https://example.com",
					"approvalToken": "approval-token",
				})
				if err != nil {
					t.Fatalf("browser approval replay did not resolve: %v", err)
				}
				if operation.RuntimeTool != "browser_open" {
					t.Fatalf("unexpected browser runtime tool: %s", operation.RuntimeTool)
				}
				if forwarded["approvalToken"] != "approval-token" {
					t.Fatalf("approval token was not forwarded: %#v", forwarded)
				}
			case "computer":
				computerFound = true
			}
		}
		if !browserFound || !computerFound {
			t.Fatalf("automation registry changed across replay iteration %d: browser=%v computer=%v", iteration, browserFound, computerFound)
		}
	}
}

func TestCompactComputerSupportsSemanticTargetAndObserve(t *testing.T) {
	var computer compactToolDef
	for _, tool := range compactAutomationToolDefinitions() {
		if tool.Name == "computer" {
			computer = tool
			break
		}
	}
	if computer.Resolve == nil {
		t.Fatal("computer compact tool not registered")
	}
	observe, _, err := computer.Resolve(map[string]any{"action": "observe"})
	if err != nil || observe.RuntimeTool != "computer_observe" {
		t.Fatalf("observe did not resolve to smart runtime operation: %#v %v", observe, err)
	}
	click, forwarded, err := computer.Resolve(map[string]any{"action": "click", "windowId": "42", "target": "Continue", "verify": true, "verifyMode": "target"})
	if err != nil || click.RuntimeTool != "computer_click" {
		t.Fatalf("semantic click did not resolve: %#v %v", click, err)
	}
	if forwarded["target"] != "Continue" || forwarded["verify"] != true || forwarded["verifyMode"] != "target" {
		t.Fatalf("semantic click arguments were not forwarded: %#v", forwarded)
	}
	if _, _, err := computer.Resolve(map[string]any{"action": "click", "windowId": "42", "target": "Continue", "verify": true, "verifyMode": "invalid"}); err == nil {
		t.Fatal("invalid computer verifyMode must be rejected")
	}
	clickByHint, forwardedHint, err := computer.Resolve(map[string]any{"action": "click", "windowHint": "Google Chrome Facebook", "target": "Notifications", "verify": true})
	if err != nil || clickByHint.RuntimeTool != "computer_click" {
		t.Fatalf("semantic click by stable window hint did not resolve: %#v %v", clickByHint, err)
	}
	if forwardedHint["windowHint"] != "Google Chrome Facebook" || forwardedHint["target"] != "Notifications" {
		t.Fatalf("stable window hint was not forwarded: %#v", forwardedHint)
	}
	focusByHint, forwardedFocus, err := computer.Resolve(map[string]any{"action": "focus", "windowHint": "Google Chrome Facebook"})
	if err != nil || focusByHint.RuntimeTool != "computer_focus" || forwardedFocus["windowHint"] != "Google Chrome Facebook" {
		t.Fatalf("focus by stable window hint did not resolve: %#v %#v %v", focusByHint, forwardedFocus, err)
	}
	if _, _, err := computer.Resolve(map[string]any{"action": "click", "target": "Continue"}); err == nil {
		t.Fatal("semantic click without windowId or windowHint must be rejected")
	}
	run, forwardedRun, err := computer.Resolve(map[string]any{
		"action": "run", "windowId": "ax:42:0",
		"steps": []any{map[string]any{"action": "click", "target": "Continue"}},
	})
	if err != nil || run.RuntimeTool != "computer_run" {
		t.Fatalf("computer run did not resolve: %#v %v", run, err)
	}
	if len(forwardedRun["steps"].([]any)) != 1 {
		t.Fatalf("computer run steps were not forwarded: %#v", forwardedRun)
	}
}

func TestAutomationRequiresProtocolV3(t *testing.T) {
	workspace := &gateway.WorkspaceView{ProtocolVersion: 2, Capabilities: map[string]any{}}
	if err := ensureAutomationOperationSupported("browser_status", workspace); err == nil {
		t.Fatal("automation must not dispatch to a protocol-v2 runtime")
	}
}

func TestAutomationImageMarkerBecomesMCPImageContent(t *testing.T) {
	imageBytes := []byte("png-bytes")
	result := toolResultWithNotice(map[string]any{
		"windowId": "42",
		"__mcpImage": map[string]any{
			"mimeType": "image/png",
			"data":     base64.StdEncoding.EncodeToString(imageBytes),
		},
	}, false, "update available")
	if result.IsError || len(result.Content) != 2 {
		t.Fatalf("unexpected MCP image result: %#v", result)
	}
	image, ok := result.Content[1].(*mcp.ImageContent)
	if !ok || image.MIMEType != "image/png" || string(image.Data) != string(imageBytes) {
		t.Fatalf("image content was not decoded: %#v", result.Content[1])
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["windowId"] != "42" {
		t.Fatalf("image metadata missing from structured content: %#v", result.StructuredContent)
	}
	if _, ok := structured["codeLocalToolSurface"].(ToolSurfaceInfo); !ok {
		t.Fatalf("image result missing tool surface metadata: %#v", result.StructuredContent)
	}
	if _, leaked := structured["__mcpImage"]; leaked {
		t.Fatal("base64 transport marker must not leak into structured content")
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text != nil && strings.Contains(text.Text, PublicToolSurface().Hash) {
		t.Fatal("tool surface hash should not be repeated in image result text")
	}
}
