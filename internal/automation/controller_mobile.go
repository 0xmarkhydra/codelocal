package automation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strings"
)

var mobileOnlyComputerTools = map[string]bool{
	"computer_list_devices":    true,
	"computer_list_apps":       true,
	"computer_launch_app":      true,
	"computer_terminate_app":   true,
	"computer_install_app":     true,
	"computer_uninstall_app":   true,
	"computer_open_url":        true,
	"computer_get_orientation": true,
	"computer_set_orientation": true,
	"computer_double_tap":      true,
	"computer_long_press":      true,
	"computer_record_start":    true,
	"computer_record_stop":     true,
	"computer_list_crashes":    true,
	"computer_get_crash":       true,
}

func isMobileComputerRequest(tool string, args map[string]any) bool {
	if tool == "computer_status" {
		return false
	}
	if mobileOnlyComputerTools[tool] {
		return true
	}
	return strings.HasPrefix(tool, "computer_") && stringArg(args, "device") != ""
}

func mobileOperation(tool string) string {
	return strings.TrimPrefix(tool, "computer_")
}

func mobileActionTarget(args map[string]any) string {
	return firstNonEmpty(
		stringArg(args, "target"), stringArg(args, "description"), stringArg(args, "packageName"),
		stringArg(args, "bundleId"), stringArg(args, "url"), stringArg(args, "path"), stringArg(args, "crashId"),
	)
}

func (c *Controller) authorizeMobile(sessionID, tool string, args map[string]any) (bool, any, error) {
	device := stringArg(args, "device")
	origin := "mobile"
	if device != "" {
		origin += ":" + device
	}
	action := Action{
		Domain: "computer", Operation: mobileOperation(tool), Origin: origin,
		Target: mobileActionTarget(args), Text: stringArg(args, "text"),
	}
	return c.authorize(sessionID, action, args)
}

func safeMobileWorkspacePath(root, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("path is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("mobile file path must stay inside the authorized workspace")
	}
	return candidate, nil
}

func mobileDirection(args map[string]any, drag bool) (string, float64, error) {
	if explicit := strings.ToLower(stringArg(args, "direction")); explicit != "" {
		switch explicit {
		case "up", "down", "left", "right":
			distance, _ := mobileNumber(args["distance"])
			return explicit, distance, nil
		default:
			return "", 0, errors.New("direction must be up, down, left, or right")
		}
	}
	var dx, dy float64
	if drag {
		fromX, fromXOK := mobileNumber(args["fromX"])
		fromY, fromYOK := mobileNumber(args["fromY"])
		toX, toXOK := mobileNumber(args["toX"])
		toY, toYOK := mobileNumber(args["toY"])
		if !fromXOK || !fromYOK || !toXOK || !toYOK {
			return "", 0, errors.New("mobile drag requires fromX, fromY, toX, and toY")
		}
		dx, dy = toX-fromX, toY-fromY
	} else {
		dx, _ = mobileNumber(args["deltaX"])
		dy, _ = mobileNumber(args["deltaY"])
	}
	if dx == 0 && dy == 0 {
		return "", 0, errors.New("mobile scroll/drag requires direction or a non-zero delta")
	}
	if math.Abs(dy) >= math.Abs(dx) {
		if drag {
			if dy < 0 {
				return "up", math.Abs(dy), nil
			}
			return "down", math.Abs(dy), nil
		}
		if dy > 0 {
			return "up", math.Abs(dy), nil
		}
		return "down", math.Abs(dy), nil
	}
	if drag {
		if dx < 0 {
			return "left", math.Abs(dx), nil
		}
		return "right", math.Abs(dx), nil
	}
	if dx > 0 {
		return "left", math.Abs(dx), nil
	}
	return "right", math.Abs(dx), nil
}

func (c *Controller) mobileTapArgs(ctx context.Context, args map[string]any) (map[string]any, map[string]any, error) {
	device := stringArg(args, "device")
	x, xOK := mobileNumber(args["x"])
	y, yOK := mobileNumber(args["y"])
	if xOK && yOK {
		return map[string]any{"device": device, "x": x, "y": y}, nil, nil
	}
	target := firstNonEmpty(stringArg(args, "target"), stringArg(args, "description"))
	if target == "" {
		return nil, nil, errors.New("mobile tap requires target or both x and y")
	}
	resolved, err := c.Mobile.FindElement(ctx, device, target)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{"device": device, "x": resolved["x"], "y": resolved["y"]}, resolved, nil
}

func (c *Controller) handleMobileComputer(ctx context.Context, tool string, args map[string]any, sessionID string) (any, error) {
	if c == nil || c.Mobile == nil {
		return nil, errors.New("Mobile Use is enabled but the managed Mobile MCP backend is not available on this CodeLocal build")
	}
	if tool != "computer_list_devices" && stringArg(args, "device") == "" {
		return nil, fmt.Errorf("%s requires device", mobileOperation(tool))
	}
	approved, state, err := c.authorizeMobile(sessionID, tool, args)
	if err != nil || !approved {
		return state, err
	}
	device := stringArg(args, "device")
	switch tool {
	case "computer_list_devices":
		return c.Mobile.Call(ctx, "mobile_list_available_devices", map[string]any{})
	case "computer_observe", "computer_ui_tree":
		value, callErr := c.Mobile.Call(ctx, "mobile_list_elements_on_screen", map[string]any{"device": device})
		if callErr != nil {
			return nil, callErr
		}
		if elements, parseErr := mobileJSONArray(mobileText(value)); parseErr == nil {
			return map[string]any{"backend": "mobile-mcp", "device": device, "elements": elements}, nil
		}
		return value, nil
	case "computer_screenshot":
		return c.Mobile.Call(ctx, "mobile_take_screenshot", map[string]any{"device": device})
	case "computer_click":
		callArgs, resolved, resolveErr := c.mobileTapArgs(ctx, args)
		if resolveErr != nil {
			return nil, resolveErr
		}
		result, callErr := c.Mobile.Call(ctx, "mobile_click_on_screen_at_coordinates", callArgs)
		if callErr != nil || resolved == nil {
			return result, callErr
		}
		return map[string]any{"result": result, "resolvedTarget": resolved, "background": true, "physicalInput": false}, nil
	case "computer_double_tap", "computer_long_press":
		callArgs, resolved, resolveErr := c.mobileTapArgs(ctx, args)
		if resolveErr != nil {
			return nil, resolveErr
		}
		mobileTool := "mobile_double_tap_on_screen"
		if tool == "computer_long_press" {
			mobileTool = "mobile_long_press_on_screen_at_coordinates"
			if duration, ok := mobileNumber(args["durationMs"]); ok && duration > 0 {
				callArgs["duration"] = duration
			}
		}
		result, callErr := c.Mobile.Call(ctx, mobileTool, callArgs)
		if callErr != nil || resolved == nil {
			return result, callErr
		}
		return map[string]any{"result": result, "resolvedTarget": resolved}, nil
	case "computer_type":
		if target := firstNonEmpty(stringArg(args, "target"), stringArg(args, "description")); target != "" {
			if _, clickErr := c.Mobile.SemanticClick(ctx, device, target); clickErr != nil {
				return nil, clickErr
			}
		}
		return c.Mobile.Call(ctx, "mobile_type_keys", map[string]any{"device": device, "text": stringArg(args, "text"), "submit": boolArg(args, "submit", false)})
	case "computer_key":
		button := strings.ToUpper(strings.ReplaceAll(stringArg(args, "key"), "-", "_"))
		return c.Mobile.Call(ctx, "mobile_press_button", map[string]any{"device": device, "button": button})
	case "computer_scroll", "computer_drag":
		direction, distance, directionErr := mobileDirection(args, tool == "computer_drag")
		if directionErr != nil {
			return nil, directionErr
		}
		callArgs := map[string]any{"device": device, "direction": direction}
		if distance > 0 {
			callArgs["distance"] = distance
		}
		if tool == "computer_drag" {
			if x, ok := mobileNumber(args["fromX"]); ok {
				callArgs["x"] = x
			}
			if y, ok := mobileNumber(args["fromY"]); ok {
				callArgs["y"] = y
			}
		} else {
			if x, ok := mobileNumber(args["x"]); ok {
				callArgs["x"] = x
			}
			if y, ok := mobileNumber(args["y"]); ok {
				callArgs["y"] = y
			}
		}
		return c.Mobile.Call(ctx, "mobile_swipe_on_screen", callArgs)
	case "computer_list_apps":
		return c.Mobile.Call(ctx, "mobile_list_apps", map[string]any{"device": device})
	case "computer_launch_app":
		callArgs := map[string]any{"device": device, "packageName": stringArg(args, "packageName")}
		if locale := stringArg(args, "locale"); locale != "" {
			callArgs["locale"] = locale
		}
		return c.Mobile.Call(ctx, "mobile_launch_app", callArgs)
	case "computer_terminate_app":
		return c.Mobile.Call(ctx, "mobile_terminate_app", map[string]any{"device": device, "packageName": stringArg(args, "packageName")})
	case "computer_install_app":
		path, pathErr := safeMobileWorkspacePath(c.Root, stringArg(args, "path"))
		if pathErr != nil {
			return nil, pathErr
		}
		return c.Mobile.Call(ctx, "mobile_install_app", map[string]any{"device": device, "path": path})
	case "computer_uninstall_app":
		return c.Mobile.Call(ctx, "mobile_uninstall_app", map[string]any{"device": device, "bundle_id": stringArg(args, "bundleId")})
	case "computer_open_url":
		rawURL := stringArg(args, "url")
		parsed, parseErr := url.Parse(rawURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, errors.New("mobile open_url only allows absolute http:// or https:// URLs")
		}
		return c.Mobile.Call(ctx, "mobile_open_url", map[string]any{"device": device, "url": rawURL})
	case "computer_get_orientation":
		return c.Mobile.Call(ctx, "mobile_get_orientation", map[string]any{"device": device})
	case "computer_set_orientation":
		return c.Mobile.Call(ctx, "mobile_set_orientation", map[string]any{"device": device, "orientation": strings.ToLower(stringArg(args, "orientation"))})
	case "computer_record_start":
		callArgs := map[string]any{"device": device}
		if output := stringArg(args, "output"); output != "" {
			safeOutput, pathErr := safeMobileWorkspacePath(c.Root, output)
			if pathErr != nil {
				return nil, pathErr
			}
			callArgs["output"] = safeOutput
		}
		if timeLimit, ok := mobileNumber(args["timeLimit"]); ok && timeLimit > 0 {
			callArgs["timeLimit"] = timeLimit
		}
		return c.Mobile.Call(ctx, "mobile_start_screen_recording", callArgs)
	case "computer_record_stop":
		return c.Mobile.Call(ctx, "mobile_stop_screen_recording", map[string]any{"device": device})
	case "computer_list_crashes":
		return c.Mobile.Call(ctx, "mobile_list_crashes", map[string]any{"device": device})
	case "computer_get_crash":
		return c.Mobile.Call(ctx, "mobile_get_crash", map[string]any{"device": device, "id": stringArg(args, "crashId")})
	default:
		return nil, errors.New("unsupported mobile computer operation: " + tool)
	}
}
