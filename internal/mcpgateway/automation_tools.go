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

func automationToolDefinitions() []toolDef {
	approval := str("One-time approval token returned by the previous approval_required result. Reuse only for the exact action that produced it.")
	origin := str("Optional current page origin for display context only. CodeLocal derives the trusted policy origin from its own browser session.")
	description := str("Short semantic description of the intended target/action. Used to detect critical actions such as payment, publishing or deletion; never include secrets.")
	return []toolDef{
		{"browser_status", "Browser status", "Show whether workspace-scoped Browser Automation is enabled, prepared and available.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"browser_open", "Open browser page", "Open an absolute http(s) URL in CodeLocal's isolated Playwright session. Localhost is allowed without external-navigation approval; remote sites follow local policy.", objectSchema(map[string]any{"url": str("Absolute http(s) URL. file:// and embedded URL credentials are blocked."), "headed": boolean("Show the managed browser window. Defaults to true."), "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "url"), false},
		{"browser_snapshot", "Inspect browser page", "Return Playwright's semantic page snapshot for the selected workspace session. Prefer this before clicks or typing.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"browser_find", "Find browser element", "Find a page element semantically and return Playwright references for later interaction.", objectSchema(map[string]any{"query": str("Semantic element query."), "workspaceKey": workspaceKeySchema}, "query"), false},
		{"browser_click", "Click browser element", "Click a Playwright element reference after local automation policy/approval.", objectSchema(map[string]any{"ref": str("Element reference from browser_snapshot/browser_find."), "origin": origin, "description": description, "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "ref"), false},
		{"browser_fill", "Fill browser field", "Fill a Playwright field reference after local automation policy/approval. Credential extraction is blocked and sensitive submission requires fresh confirmation.", objectSchema(map[string]any{"ref": str("Field reference from browser_snapshot/browser_find."), "text": str("Text to enter."), "origin": origin, "description": description, "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "ref", "text"), false},
		{"browser_press", "Press browser key", "Press a keyboard key in the managed browser after local automation policy/approval.", objectSchema(map[string]any{"key": str("Playwright key name such as Enter, Escape or ArrowDown."), "origin": origin, "description": description, "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "key"), false},
		{"browser_screenshot", "Capture browser screenshot", "Capture the current page or one referenced element and return a real MCP image content block after screenshot approval.", objectSchema(map[string]any{"ref": str("Optional element reference."), "origin": origin, "description": description, "approvalToken": approval, "workspaceKey": workspaceKeySchema}), false},
		{"browser_console", "Read browser console", "Read console output from the managed Playwright session.", objectSchema(map[string]any{"level": str("Optional console level filter."), "workspaceKey": workspaceKeySchema}), false},
		{"browser_requests", "Read browser requests", "Inspect network requests observed by the managed Playwright session.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"browser_close", "Close browser session", "Close the workspace-scoped managed browser session. This never closes the user's normal browser profile.", objectSchema(map[string]any{"approvalToken": approval, "workspaceKey": workspaceKeySchema}), false},

		{"computer_status", "Computer Use status", "Show the detected native Computer Use backend and granular capabilities. A capability is never reported available until the packaged native helper reports it ready.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"computer_list_windows", "List desktop windows", "List visible application windows through the native accessibility backend.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"computer_ui_tree", "Inspect desktop UI", "Read a structured accessibility/UI Automation tree before using coordinate fallback.", objectSchema(map[string]any{"windowId": str("Optional window identifier returned by computer_list_windows."), "workspaceKey": workspaceKeySchema}), false},
		{"computer_screenshot", "Capture desktop screenshot", "Capture a screen or window through the OS-native helper after local approval.", objectSchema(map[string]any{"windowId": str("Optional window identifier."), "description": description, "approvalToken": approval, "workspaceKey": workspaceKeySchema}), false},
		{"computer_focus", "Focus desktop window", "Focus a desktop application/window through the native helper.", computerActionSchema(approval, description, map[string]any{"windowId": str("Window identifier.")}, "windowId"), false},
		{"computer_click", "Click desktop UI", "Invoke/click a structured UI target when possible, otherwise use approved coordinates.", computerActionSchema(approval, description, map[string]any{"windowId": str("Window identifier."), "elementId": str("Accessibility element identifier."), "x": integer("Fallback screen X coordinate.", 0, 0), "y": integer("Fallback screen Y coordinate.", 0, 0)}), false},
		{"computer_type", "Type desktop text", "Type text into the focused/selected desktop element after local approval.", computerActionSchema(approval, description, map[string]any{"text": str("Text to type."), "elementId": str("Optional accessibility element identifier.")}, "text"), false},
		{"computer_key", "Press desktop key", "Press an OS-level keyboard key/shortcut after local approval.", computerActionSchema(approval, description, map[string]any{"key": str("Key or shortcut.")}, "key"), false},
		{"computer_scroll", "Scroll desktop UI", "Scroll the selected window/element after local approval.", computerActionSchema(approval, description, map[string]any{"deltaX": integer("Horizontal scroll delta.", 0, 0), "deltaY": integer("Vertical scroll delta.", 0, 0)}), false},
		{"computer_drag", "Drag desktop UI", "Drag between approved coordinates/elements. Secure desktop and privilege prompts remain out of scope.", computerActionSchema(approval, description, map[string]any{"fromX": integer("Start X coordinate.", 0, 0), "fromY": integer("Start Y coordinate.", 0, 0), "toX": integer("End X coordinate.", 0, 0), "toY": integer("End Y coordinate.", 0, 0)}), false},
	}
}

func computerActionSchema(approval, description map[string]any, extra map[string]any, required ...string) json.RawMessage {
	properties := map[string]any{"description": description, "approvalToken": approval, "workspaceKey": workspaceKeySchema}
	for key, value := range extra {
		properties[key] = value
	}
	return objectSchema(properties, required...)
}

func automationTool(name string) bool {
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
	case "computer_list_windows", "computer_ui_tree":
		return "uiTree"
	case "computer_screenshot":
		return "screenCapture"
	case "computer_focus":
		return "uiTree"
	case "computer_click", "computer_scroll", "computer_drag":
		return "pointer"
	case "computer_type", "computer_key":
		return "keyboard"
	default:
		return ""
	}
}

func ensureAutomationToolSupported(def toolDef, workspace *gateway.WorkspaceView) error {
	if workspace == nil {
		return errors.New("workspace unavailable")
	}
	if workspace.ProtocolVersion < 3 {
		return fmt.Errorf("%s requires CodeLocal automation protocol v3; update the local codelocal package", def.Name)
	}
	// Status tools stay callable so ChatGPT can explain why a capability is not ready.
	if def.Name == "browser_status" || def.Name == "computer_status" {
		return nil
	}
	automation := nestedMap(workspace.Capabilities["automation"])
	domain := "browser"
	if strings.HasPrefix(def.Name, "computer_") {
		domain = "computer"
	}
	capability := nestedMap(automation[domain])
	if !capabilityFlag(capability, "available") {
		return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise %s automation support", def.Name, domain)
	}
	if domain == "computer" {
		required := requiredComputerCapability(def.Name)
		if required != "" && !capabilityFlag(capability, required) {
			return fmt.Errorf("%s is unavailable because the native Computer Use backend does not advertise %s support", def.Name, required)
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
	return &mcp.CallToolResult{Content: content, IsError: isError}
}
