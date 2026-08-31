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

func generationFiveCompactAutomationToolDefinitions() []compactToolDef {
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

func compactAutomationToolDefinitions() []compactToolDef {
	defs := generationFiveCompactAutomationToolDefinitions()
	for index := range defs {
		if defs[index].Name == "computer" {
			defs[index] = mobileAwareComputerToolDefinition(defs[index])
		}
	}
	return defs
}

func mobileAwareComputerToolDefinition(base compactToolDef) compactToolDef {
	actions := map[string]string{
		"status": "computer_status", "devices": "computer_list_devices", "observe": "computer_observe", "list_windows": "computer_list_windows", "ui_tree": "computer_ui_tree",
		"screenshot": "computer_screenshot", "focus": "computer_focus", "click": "computer_click", "double_tap": "computer_double_tap", "long_press": "computer_long_press",
		"type": "computer_type", "key": "computer_key", "scroll": "computer_scroll", "drag": "computer_drag", "run": "computer_run",
		"apps": "computer_list_apps", "launch_app": "computer_launch_app", "terminate_app": "computer_terminate_app", "install_app": "computer_install_app", "uninstall_app": "computer_uninstall_app",
		"open_url": "computer_open_url", "orientation": "computer_get_orientation", "set_orientation": "computer_set_orientation", "record_start": "computer_record_start", "record_stop": "computer_record_stop",
		"crashes": "computer_list_crashes", "crash": "computer_get_crash",
	}
	approval := str("One-time approval token returned by the previous approval-required result. Reuse only for the exact action that produced it.")
	base.Title = "Use desktop and mobile apps"
	base.Description = "Observe and control native desktop apps plus connected iOS/Android devices through one Computer tool. Omit device for desktop; use action=devices then pass device for mobile. Prefer semantic target text over coordinates. Mobile MCP is a managed internal backend, not an extra public MCP tool. App lifecycle, recording, URL opening and device input remain approval-controlled."
	base.Schema = actionSchema(
		[]string{"status", "devices", "observe", "list_windows", "ui_tree", "screenshot", "focus", "click", "double_tap", "long_press", "type", "key", "scroll", "drag", "run", "apps", "launch_app", "terminate_app", "install_app", "uninstall_app", "open_url", "orientation", "set_orientation", "record_start", "record_stop", "crashes", "crash"},
		map[string]any{
			"device":        str("Mobile device identifier from action=devices. Omit for desktop Computer Use."),
			"windowId":      str("Desktop window identifier returned by observe/list_windows."),
			"windowHint":    str("Stable desktop app/title hint used to resolve a fresh windowId."),
			"elementId":     str("Desktop accessibility element identifier. Prefer target."),
			"target":        str("Semantic UI target such as Continue, Save, or Login. On mobile CodeLocal resolves this against the fresh accessibility element list."),
			"verify":        boolean("After a desktop input action, verify the result in the same call. Mobile actions already resolve against fresh device state when semantic targeting is used."),
			"verifyMode":    str("Desktop verification mode: target, scene, or none."),
			"x":             integer("Screen X coordinate fallback or mobile swipe start X.", 0, 0),
			"y":             integer("Screen Y coordinate fallback or mobile swipe start Y.", 0, 0),
			"text":          str("Text to type into the focused/selected element."),
			"submit":        boolean("For mobile typing, press Enter after text input."),
			"key":           str("Desktop key name or mobile button such as BACK, HOME, ENTER, VOLUME_UP, or VOLUME_DOWN."),
			"deltaX":        integer("Horizontal scroll delta.", 0, 0),
			"deltaY":        integer("Vertical scroll delta.", 0, 0),
			"fromX":         integer("Drag/swipe start X coordinate.", 0, 0),
			"fromY":         integer("Drag/swipe start Y coordinate.", 0, 0),
			"toX":           integer("Drag/swipe end X coordinate.", 0, 0),
			"toY":           integer("Drag/swipe end Y coordinate.", 0, 0),
			"direction":     map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}, "description": "Mobile swipe direction. Can replace delta/drag coordinates."},
			"distance":      integer("Optional mobile swipe distance in pixels.", 1, 10000),
			"durationMs":    integer("Optional mobile long-press duration in milliseconds.", 1, 10000),
			"steps":         array(anyObject("Desktop background sequence step. Each step uses click or type with a semantic target."), "Bounded desktop semantic steps for action=run."),
			"packageName":   str("Mobile app package name/bundle identifier used by launch_app or terminate_app."),
			"bundleId":      str("Mobile app bundle ID/package name used by uninstall_app."),
			"path":          str("Workspace-relative .apk/.ipa/.app/.zip path used by install_app."),
			"locale":        str("Optional comma-separated BCP 47 locale tags for launch_app."),
			"url":           str("Absolute http(s) URL to open on the mobile device. Unsafe custom schemes stay blocked."),
			"orientation":   map[string]any{"type": "string", "enum": []string{"portrait", "landscape"}, "description": "Mobile screen orientation."},
			"output":        str("Optional workspace-relative .mp4 path for mobile screen recording."),
			"timeLimit":     integer("Optional mobile screen recording limit in seconds.", 1, 7200),
			"crashId":       str("Mobile crash report ID returned by action=crashes."),
			"description":   str("Short semantic description of the target/action. Never include secrets."),
			"approvalToken": approval,
		},
	)
	base.Annotations = compactAnnotations("Use desktop and mobile apps", false, true, true)
	base.Resolve = func(args map[string]any) (operationInvocation, map[string]any, error) {
		operation, forward, err := resolveAction(args, actions, map[string][]string{
			"type": {"text"}, "key": {"key"}, "run": {"windowId", "steps"},
			"apps": {"device"}, "launch_app": {"device", "packageName"}, "terminate_app": {"device", "packageName"}, "install_app": {"device", "path"},
			"uninstall_app": {"device", "bundleId"}, "open_url": {"device", "url"}, "orientation": {"device"}, "set_orientation": {"device", "orientation"},
			"record_start": {"device"}, "record_stop": {"device"}, "crashes": {"device"}, "crash": {"device", "crashId"},
		})
		if err != nil {
			return operationInvocation{}, nil, err
		}
		action, _ := args["action"].(string)
		action = strings.TrimSpace(action)
		device, _ := forward["device"].(string)
		device = strings.TrimSpace(device)
		mobile := device != "" || action == "devices" || action == "apps" || action == "launch_app" || action == "terminate_app" || action == "install_app" || action == "uninstall_app" || action == "open_url" || action == "orientation" || action == "set_orientation" || action == "double_tap" || action == "long_press" || action == "record_start" || action == "record_stop" || action == "crashes" || action == "crash"
		if rawMode, exists := forward["verifyMode"]; exists && !mobile {
			mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawMode)))
			if mode != "" && mode != "target" && mode != "scene" && mode != "none" {
				return operationInvocation{}, nil, errors.New("verifyMode must be target, scene, or none")
			}
		}
		if action == "focus" {
			if mobile {
				return operationInvocation{}, nil, errors.New("focus is desktop-only; mobile input targets the selected device directly")
			}
			windowID, _ := forward["windowId"].(string)
			windowHint, _ := forward["windowHint"].(string)
			if strings.TrimSpace(windowID) == "" && strings.TrimSpace(windowHint) == "" {
				return operationInvocation{}, nil, errors.New("focus requires windowId or windowHint")
			}
		}
		if action == "click" || action == "double_tap" || action == "long_press" {
			target, _ := forward["target"].(string)
			description, _ := forward["description"].(string)
			_, hasX := forward["x"]
			_, hasY := forward["y"]
			semantic := strings.TrimSpace(firstNonEmptyString(target, description))
			if semantic == "" && (!hasX || !hasY) {
				return operationInvocation{}, nil, fmt.Errorf("%s requires target or both x and y", action)
			}
			if !mobile && action == "click" {
				elementID, _ := forward["elementId"].(string)
				if strings.TrimSpace(elementID) == "" && semantic != "" {
					windowID, _ := forward["windowId"].(string)
					windowHint, _ := forward["windowHint"].(string)
					if strings.TrimSpace(windowID) == "" && strings.TrimSpace(windowHint) == "" {
						return operationInvocation{}, nil, errors.New("semantic desktop click requires windowId or stable windowHint")
					}
				}
			}
		}
		if (action == "double_tap" || action == "long_press") && device == "" {
			return operationInvocation{}, nil, fmt.Errorf("%s requires device", action)
		}
		if action == "run" {
			if device != "" {
				return operationInvocation{}, nil, errors.New("run is desktop-only; issue mobile actions individually so each remains approval-scoped")
			}
			steps, ok := forward["steps"].([]any)
			if !ok || len(steps) == 0 {
				return operationInvocation{}, nil, errors.New("run requires at least one semantic step")
			}
			if len(steps) > 12 {
				return operationInvocation{}, nil, errors.New("run supports at most 12 semantic steps")
			}
		}
		if action == "drag" && device == "" {
			if err := requireArgs(forward, action, "fromX", "fromY", "toX", "toY"); err != nil {
				return operationInvocation{}, nil, err
			}
		}
		if action == "ui_tree" && device == "" {
			if _, ok := forward["windowId"]; !ok {
				return operationInvocation{}, nil, errors.New("desktop ui_tree requires windowId")
			}
		}
		if action == "list_windows" && device != "" {
			return operationInvocation{}, nil, errors.New("list_windows is desktop-only; use observe for mobile UI state")
		}
		return operation, forward, nil
	}
	return base
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

func mobileOnlyRuntimeTool(tool string) bool {
	switch tool {
	case "computer_list_devices", "computer_list_apps", "computer_launch_app", "computer_terminate_app", "computer_install_app", "computer_uninstall_app", "computer_open_url", "computer_get_orientation", "computer_set_orientation", "computer_double_tap", "computer_long_press", "computer_record_start", "computer_record_stop", "computer_list_crashes", "computer_get_crash":
		return true
	default:
		return false
	}
}

func mobileRuntimeRequest(tool string, args map[string]any) bool {
	if mobileOnlyRuntimeTool(tool) {
		return true
	}
	device, _ := args["device"].(string)
	return strings.HasPrefix(tool, "computer_") && strings.TrimSpace(device) != ""
}

func requiredMobileCapability(tool string) string {
	switch tool {
	case "computer_list_devices":
		return "deviceList"
	case "computer_observe", "computer_ui_tree":
		return "uiTree"
	case "computer_screenshot":
		return "screenCapture"
	case "computer_click", "computer_double_tap", "computer_long_press", "computer_scroll", "computer_drag":
		return "pointer"
	case "computer_type", "computer_key":
		return "keyboard"
	case "computer_list_apps", "computer_launch_app", "computer_terminate_app", "computer_install_app", "computer_uninstall_app":
		return "appLifecycle"
	case "computer_open_url":
		return "openURL"
	case "computer_get_orientation", "computer_set_orientation":
		return "orientation"
	case "computer_record_start", "computer_record_stop":
		return "recording"
	case "computer_list_crashes", "computer_get_crash":
		return "crashReports"
	default:
		return ""
	}
}

func ensureAutomationOperationSupported(runtimeTool string, workspace *gateway.WorkspaceView, optionalArgs ...map[string]any) error {
	var args map[string]any
	if len(optionalArgs) > 0 {
		args = optionalArgs[0]
	}
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
	if domain == "computer" && mobileRuntimeRequest(runtimeTool, args) {
		mobile := nestedMap(capability["mobile"])
		if !capabilityFlag(mobile, "available") {
			return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise managed mobile automation support", runtimeTool)
		}
		if required := requiredMobileCapability(runtimeTool); required != "" && !capabilityFlag(mobile, required) {
			return fmt.Errorf("%s is unavailable because the managed mobile backend does not advertise %s support", runtimeTool, required)
		}
		return nil
	}
	if !capabilityFlag(capability, "available") {
		return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise %s automation support", runtimeTool, domain)
	}
	if domain == "computer" {
		if _, explicit := capability["desktopAvailable"]; explicit && !capabilityFlag(capability, "desktopAvailable") {
			return fmt.Errorf("%s is unavailable because this CodeLocal client has no desktop Computer Use backend", runtimeTool)
		}
		required := requiredComputerCapability(runtimeTool)
		if required != "" && !capabilityFlag(capability, required) {
			return fmt.Errorf("%s is unavailable because the native Computer Use backend does not advertise %s support", runtimeTool, required)
		}
	}
	return nil
}

func linkedImageToolResult(root map[string]any, isError bool, notice string) *mcp.CallToolResult {
	marker := nestedMap(root["__mcpImageRef"])
	uri, _ := marker["url"].(string)
	mimeType, _ := marker["mimeType"].(string)
	if strings.TrimSpace(uri) == "" || strings.TrimSpace(mimeType) == "" {
		return nil
	}
	metadata := map[string]any{}
	for key, item := range root {
		if key != "__mcpImageRef" && key != "__mcpImage" {
			metadata[key] = item
		}
	}
	metadata["visual"] = marker
	metadata["codeLocalToolSurface"] = PublicToolSurface()
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
	var size *int64
	switch raw := marker["size"].(type) {
	case float64:
		value := int64(raw)
		size = &value
	case int:
		value := int64(raw)
		size = &value
	case int64:
		value := raw
		size = &value
	}
	content = append(content, &mcp.ResourceLink{
		URI: uri, Name: "CodeLocal visual", Title: "CodeLocal visual", MIMEType: mimeType, Size: size,
	})
	return &mcp.CallToolResult{Content: content, StructuredContent: metadata, IsError: isError}
}

func toolResultWithNotice(value any, isError bool, notice string) *mcp.CallToolResult {
	root, ok := value.(map[string]any)
	if !ok {
		return textResultWithNotice(value, isError, notice)
	}
	if linked := linkedImageToolResult(root, isError, notice); linked != nil {
		return linked
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
