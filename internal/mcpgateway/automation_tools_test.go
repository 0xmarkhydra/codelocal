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
	workspace.Capabilities["automation"].(map[string]any)["computer"].(map[string]any)["pointer"] = true
	if err := ensureAutomationOperationSupported("computer_click", workspace); err != nil {
		t.Fatalf("advertised pointer capability should pass: %v", err)
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
	click, forwarded, err := computer.Resolve(map[string]any{"action": "click", "windowId": "42", "target": "Continue", "verify": true})
	if err != nil || click.RuntimeTool != "computer_click" {
		t.Fatalf("semantic click did not resolve: %#v %v", click, err)
	}
	if forwarded["target"] != "Continue" || forwarded["verify"] != true {
		t.Fatalf("semantic click arguments were not forwarded: %#v", forwarded)
	}
	if _, _, err := computer.Resolve(map[string]any{"action": "click", "target": "Continue"}); err == nil {
		t.Fatal("semantic click without a windowId must be rejected")
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
	if _, leaked := structured["__mcpImage"]; leaked {
		t.Fatal("base64 transport marker must not leak into structured content")
	}
}
