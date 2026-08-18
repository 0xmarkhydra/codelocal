package automation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	computerVerifyNone   = "none"
	computerVerifyTarget = "target"
	computerVerifyScene  = "scene"
)

func computerVerificationMode(args map[string]any, targetable bool) string {
	if !boolArg(args, "verify", false) {
		return computerVerifyNone
	}
	mode := strings.ToLower(strings.TrimSpace(stringArg(args, "verifyMode")))
	switch mode {
	case computerVerifyNone:
		return computerVerifyNone
	case computerVerifyTarget:
		if targetable {
			return computerVerifyTarget
		}
		return computerVerifyScene
	case computerVerifyScene:
		return computerVerifyScene
	case "":
		if targetable {
			return computerVerifyTarget
		}
		return computerVerifyScene
	default:
		if targetable {
			return computerVerifyTarget
		}
		return computerVerifyScene
	}
}

func resolvedTargetFromActionResult(value any) map[string]any {
	root, _ := value.(map[string]any)
	if root == nil {
		return nil
	}
	resolved, _ := root["resolvedTarget"].(map[string]any)
	return resolved
}

func resolvedElementID(value any) string {
	resolved := resolvedTargetFromActionResult(value)
	if resolved == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(resolved["elementId"]))
}

func (c *ComputerController) ElementRead(ctx context.Context, windowID, elementID string) (any, error) {
	return c.Call(ctx, "element_read", map[string]any{
		"windowId":  strings.TrimSpace(windowID),
		"elementId": strings.TrimSpace(elementID),
	})
}

func verificationElementSnapshot(operation, typedText string, value any) map[string]any {
	root, _ := value.(map[string]any)
	if root == nil {
		return map[string]any{"present": false}
	}
	out := map[string]any{"present": true}
	for _, key := range []string{"elementId", "role", "name", "description", "enabled", "bounds", "engine"} {
		if item, ok := root[key]; ok {
			out[key] = item
		}
	}
	if operation == "type" {
		if raw, ok := root["value"]; ok {
			actual := fmt.Sprint(raw)
			out["valueMatches"] = actual == typedText
			out["valueLength"] = len([]rune(actual))
		}
		return out
	}
	if raw, ok := root["value"]; ok {
		out["value"] = raw
	}
	return out
}

func verifyComputerAction(ctx context.Context, computer *ComputerController, operation, windowID, target, typedText string, actionResult any, mode string) (any, error) {
	if computer == nil || mode == computerVerifyNone {
		return nil, nil
	}
	if mode == computerVerifyScene {
		return ObserveComputer(ctx, computer, windowID)
	}

	started := time.Now()
	elementID := resolvedElementID(actionResult)
	if elementID == "" {
		// Backends that cannot return a durable semantic element identity retain
		// the legacy scene verification path rather than pretending target-level
		// verification succeeded.
		return ObserveComputer(ctx, computer, windowID)
	}

	element, err := computer.ElementRead(ctx, windowID, elementID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unsupported") {
			return ObserveComputer(ctx, computer, windowID)
		}
		observation := map[string]any{
			"mode":          computerVerifyTarget,
			"windowId":      windowID,
			"target":        target,
			"elementId":     elementID,
			"targetPresent": false,
			"targetError":   err.Error(),
			"scene":         computer.SceneMetadata(windowID),
			"durationMs":    time.Since(started).Milliseconds(),
		}
		return observation, fmt.Errorf("target verification failed: %w", err)
	}

	snapshot := verificationElementSnapshot(operation, typedText, element)
	observation := map[string]any{
		"mode":       computerVerifyTarget,
		"windowId":   windowID,
		"target":     target,
		"element":    snapshot,
		"scene":      computer.SceneMetadata(windowID),
		"durationMs": time.Since(started).Milliseconds(),
	}
	if operation == "type" {
		matches, readable := snapshot["valueMatches"].(bool)
		observation["verified"] = readable && matches
		if !readable {
			observation["verificationUncertain"] = true
		}
	}
	return observation, nil
}

func (c *Controller) attachComputerVerification(ctx context.Context, envelope map[string]any, mode, operation, windowID, target, typedText string, actionResult any) {
	if c == nil || c.Computer == nil || envelope == nil || mode == computerVerifyNone {
		return
	}
	started := time.Now()
	observation, err := verifyComputerAction(ctx, c.Computer, operation, windowID, target, typedText, actionResult, mode)
	envelope["verificationMode"] = mode
	envelope["verifyMs"] = time.Since(started).Milliseconds()
	if observation != nil {
		envelope["observation"] = observation
	}
	if err != nil {
		envelope["verificationError"] = err.Error()
		return
	}
	if operation == "type" && mode == computerVerifyTarget {
		if root, ok := observation.(map[string]any); ok {
			if verified, ok := root["verified"].(bool); !ok || !verified {
				envelope["verificationError"] = "typed value could not be confirmed after background update"
			}
		}
	}
}
