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
		"status": "computer_status", "observe": "computer_observe", "list_windows": "computer_list_windows", "ui_tree": "computer_ui_tree",
		"screenshot": "computer_screenshot", "focus": "computer_focus", "click": "computer_click", "type": "computer_type",
		"key": "computer_key", "scroll": "computer_scroll", "drag": "computer_drag", "run": "computer_run",
	}

	return []compactToolDef{
		{
			Name:        "browser",
			Title:       "Automate websites",
			Description: "Open and inspect an isolated Chromium session, then find/click/fill/press elements, capture screenshots, read console/network activity, or close the session. Prefer snapshot/find before interaction. Set verify=true on click/fill/press to return a fresh post-action snapshot in the same call.",
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
					"verify":        boolean("After click/fill/press, include a fresh browser snapshot in the same MCP call."),
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
			Description: "Observe desktop windows/UI and interact with native apps. Prefer action=observe, then semantic target text for click. action=run executes a bounded same-window sequence of semantic background click/type steps in one MCP round trip when the client advertises batchActions. Raw coordinates remain a last-resort single action. Set verify=true for post-action verification; semantic actions default to targeted element verification, while verifyMode=scene requests the full window observation.",
			Schema: actionSchema(
				[]string{"status", "observe", "list_windows", "ui_tree", "screenshot", "focus", "click", "type", "key", "scroll", "drag", "run"},
				map[string]any{
					"windowId":      str("Window identifier returned by action=observe or action=list_windows."),
					"windowHint":    str("Stable app/title hint used to resolve a fresh windowId, especially when replaying a learned skill."),
					"elementId":     str("Accessibility element identifier returned by action=ui_tree. Usually omit this and provide target instead."),
					"target":        str("Semantic UI target such as Continue or Save. For click, CodeLocal resolves this against a fresh accessibility tree when elementId is omitted."),
					"verify":        boolean("After an input action, verify the result in the same MCP call. Semantic actions default to lightweight target verification. Defaults to false."),
					"verifyMode":    str("Optional verification mode: target for the acted element or scene for a full window observation."),
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
					"steps":         array(anyObject("Background sequence step. Each step must use action=click or action=type with a semantic target; type also requires text. All steps inherit the top-level windowId."), "Bounded semantic background steps for action=run."),
					"description":   description,
					"approvalToken": approval,
				},
			),
			Annotations: compactAnnotations("Use desktop apps", false, true, true),
			Resolve: func(args map[string]any) (operationInvocation, map[string]any, error) {
				operation, forward, err := resolveAction(args, computerActions, map[string][]string{
					"type": {"text"}, "key": {"key"},
					"drag": {"fromX", "fromY", "toX", "toY"}, "run": {"windowId", "steps"},
				})
				if err != nil {
					return operationInvocation{}, nil, err
				}
				if rawMode, exists := forward["verifyMode"]; exists {
					mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawMode)))
					if mode != "" && mode != "target" && mode != "scene" && mode != "none" {
						return operationInvocation{}, nil, errors.New("verifyMode must be target, scene, or none")
					}
				}
				action, _ := args["action"].(string)
				if action == "focus" {
					windowID, _ := forward["windowId"].(string)
					windowHint, _ := forward["windowHint"].(string)
					if strings.TrimSpace(windowID) == "" && strings.TrimSpace(windowHint) == "" {
						return operationInvocation{}, nil, errors.New("focus requires windowId or windowHint")
					}
				}
				if action == "click" {
					elementID, _ := forward["elementId"].(string)
					target, _ := forward["target"].(string)
					description, _ := forward["description"].(string)
					_, hasX := forward["x"]
					_, hasY := forward["y"]
					semantic := strings.TrimSpace(firstNonEmptyString(target, description))
					if strings.TrimSpace(elementID) == "" && semantic == "" && (!hasX || !hasY) {
						return operationInvocation{}, nil, errors.New("click requires target, elementId, or both x and y")
					}
					if strings.TrimSpace(elementID) == "" && semantic != "" {
						windowID, _ := forward["windowId"].(string)
						windowHint, _ := forward["windowHint"].(string)
						if strings.TrimSpace(windowID) == "" && strings.TrimSpace(windowHint) == "" {
							return operationInvocation{}, nil, errors.New("semantic click requires windowId or stable windowHint")
						}
					}
				}
				if action == "run" {
					steps, ok := forward["steps"].([]any)
					if !ok || len(steps) == 0 {
						return operationInvocation{}, nil, errors.New("run requires at least one semantic step")
					}
					if len(steps) > 12 {
						return operationInvocation{}, nil, errors.New("run supports at most 12 semantic steps")
					}
				}
				return operation, forward, nil
			},
		},
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
	case "computer_observe", "computer_list_windows", "computer_focus":
		return "windowList"
	case "computer_ui_tree":
		return "uiTree"
	case "computer_run":
		return "batchActions"
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
	metadata["codeLocalToolSurface"] = PublicToolSurface()
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
