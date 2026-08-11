package mcpgateway

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func compactAutomationToolDefinitions() []compactToolDef {
	approval := str("One-time approval token returned by the previous approval-required result. Reuse only for the exact action that produced it.")
	description := str("Short semantic description of the target/action. Never include secrets.")
	browserActions := map[string]string{
		"status": "browser_status", "open": "browser_open", "snapshot": "browser_snapshot", "find": "browser_find",
		"click": "browser_click", "fill": "browser_fill", "press": "browser_press", "screenshot": "browser_screenshot",
		"console": "browser_console", "requests": "browser_requests", "close": "browser_close",
	}
	computerActions := map[string]string{
		"status": "computer_status", "list_windows": "computer_list_windows", "ui_tree": "computer_ui_tree",
		"screenshot": "computer_screenshot", "focus": "computer_focus", "click": "computer_click", "type": "computer_type",
		"key": "computer_key", "scroll": "computer_scroll", "drag": "computer_drag",
	}

	return []compactToolDef{
		{
			Name:        "browser",
			Title:       "Automate websites",
			Description: "Open and inspect an isolated Chromium session, then find/click/fill/press elements, capture screenshots, read console/network activity, or close the session. Prefer snapshot/find before interaction.",
			Schema: actionSchema(
				[]string{"status", "open", "snapshot", "find", "click", "fill", "press", "screenshot", "console", "requests", "close"},
				map[string]any{
					"url":           str("Absolute http(s) URL. file:// and embedded URL credentials are blocked."),
					"headed":        boolean("Show the managed browser window. Defaults to true."),
					"query":         str("Semantic element query."),
					"ref":           str("Element reference from snapshot/find."),
					"text":          str("Text to enter."),
					"key":           str("Playwright key name such as Enter, Escape or ArrowDown."),
					"level":         str("Optional console level filter."),
					"description":   description,
					"approvalToken": approval,
				},
			),
			Annotations: compactAnnotations("Automate websites", false, true, true),
			Resolve: func(args map[string]any) (operationInvocation, map[string]any, error) {
				return resolveAction(args, browserActions, map[string][]string{
					"open": {"url"}, "find": {"query"}, "click": {"ref"}, "fill": {"ref", "text"}, "press": {"key"},
				})
			},
		},
		{
			Name:        "computer",
			Title:       "Use desktop apps",
			Description: "Inspect desktop windows/UI, capture a fresh screenshot, focus a window, or perform approval-controlled pointer and keyboard actions. Prefer elementId over coordinates and re-inspect after UI changes.",
			Schema: actionSchema(
				[]string{"status", "list_windows", "ui_tree", "screenshot", "focus", "click", "type", "key", "scroll", "drag"},
				map[string]any{
					"windowId":      str("Window identifier returned by action=list_windows."),
					"elementId":     str("Accessibility element identifier returned by action=ui_tree."),
					"x":             integer("Fallback screen X coordinate.", 0, 0),
					"y":             integer("Fallback screen Y coordinate.", 0, 0),
					"text":          str("Text to type into the focused/selected element."),
					"key":           str("OS-level key name or supported shortcut."),
					"deltaX":        integer("Horizontal scroll delta.", 0, 0),
					"deltaY":        integer("Vertical scroll delta.", 0, 0),
					"fromX":         integer("Drag start X coordinate.", 0, 0),
					"fromY":         integer("Drag start Y coordinate.", 0, 0),
					"toX":           integer("Drag end X coordinate.", 0, 0),
					"toY":           integer("Drag end Y coordinate.", 0, 0),
					"description":   description,
					"approvalToken": approval,
				},
			),
			Annotations: compactAnnotations("Use desktop apps", false, true, false),
			Resolve: func(args map[string]any) (operationInvocation, map[string]any, error) {
				operation, forward, err := resolveAction(args, computerActions, map[string][]string{
					"focus": {"windowId"}, "type": {"text"}, "key": {"key"},
					"drag": {"fromX", "fromY", "toX", "toY"},
				})
				if err != nil {
					return operationInvocation{}, nil, err
				}
				action, _ := args["action"].(string)
				if action == "click" {
					elementID, _ := forward["elementId"].(string)
					_, hasX := forward["x"]
					_, hasY := forward["y"]
					if strings.TrimSpace(elementID) == "" && (!hasX || !hasY) {
						return operationInvocation{}, nil, errors.New("click requires elementId or both x and y")
					}
				}
				return operation, forward, nil
			},
		},
	}
}

func automationRuntimeTool(name string) bool {
	return strings.HasPrefix(name, "browser_") || strings.HasPrefix(name, "computer_")
}

func nestedMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func capabilityFlag(capability map[string]any, name string) bool {
	value, _ := capability[name].(bool)
	return value
}

func requiredComputerCapability(tool string) string {
	switch tool {
	case "computer_list_windows", "computer_focus":
		return "windowList"
	case "computer_ui_tree":
		return "uiTree"
	case "computer_screenshot":
		return "screenCapture"
	case "computer_click", "computer_scroll", "computer_drag":
		return "pointer"
	case "computer_type", "computer_key":
		return "keyboard"
	default:
		return ""
	}
}

func ensureAutomationOperationSupported(runtimeTool string, workspace *gateway.WorkspaceView) error {
	if workspace == nil {
		return errors.New("workspace unavailable")
	}
	if workspace.ProtocolVersion < 3 {
		return fmt.Errorf("%s requires CodeLocal automation protocol v3; update the local codelocal package", runtimeTool)
	}
	// Status remains callable so ChatGPT can explain why a capability is unavailable.
	if runtimeTool == "browser_status" || runtimeTool == "computer_status" {
		return nil
	}
	automation := nestedMap(workspace.Capabilities["automation"])
	domain := "browser"
	if strings.HasPrefix(runtimeTool, "computer_") {
		domain = "computer"
	}
	capability := nestedMap(automation[domain])
	if !capabilityFlag(capability, "available") {
		return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise %s automation support", runtimeTool, domain)
	}
	if domain == "computer" {
		required := requiredComputerCapability(runtimeTool)
		if required != "" && !capabilityFlag(capability, required) {
			return fmt.Errorf("%s is unavailable because the native Computer Use backend does not advertise %s support", runtimeTool, required)
		}
	}
	return nil
}

func toolResultWithNotice(value any, isError bool, notice string) *mcp.CallToolResult {
	root, ok := value.(map[string]any)
	if !ok {
		return textResultWithNotice(value, isError, notice)
	}
	marker := nestedMap(root["__mcpImage"])
	encoded, _ := marker["data"].(string)
	mimeType, _ := marker["mimeType"].(string)
	if encoded == "" || mimeType == "" {
		return textResultWithNotice(value, isError, notice)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return textResultWithNotice(value, isError, notice)
	}
	metadata := map[string]any{}
	for key, item := range root {
		if key != "__mcpImage" {
			metadata[key] = item
		}
	}
	var text string
	if raw, err := json.MarshalIndent(metadata, "", "  "); err == nil {
		text = string(raw)
	}
	if strings.TrimSpace(notice) != "" {
		text = notice + "\n\n" + text
	}
	content := []mcp.Content{}
	if strings.TrimSpace(text) != "" && text != "{}" {
		content = append(content, &mcp.TextContent{Text: text})
	}
	content = append(content, &mcp.ImageContent{Data: data, MIMEType: mimeType})
	return &mcp.CallToolResult{Content: content, StructuredContent: metadata, IsError: isError}
}
