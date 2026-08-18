package automation

import (
	"context"
	"fmt"
	"strings"
)

func computerElementSensitive(metadata map[string]any, target string) bool {
	if metadata == nil {
		return true
	}
	role := strings.ToLower(strings.TrimSpace(fmt.Sprint(metadata["role"])))
	if role == "" {
		return true
	}
	if strings.Contains(role, "secure") || strings.Contains(role, "password") {
		return true
	}
	name := strings.TrimSpace(fmt.Sprint(metadata["name"]))
	description := strings.TrimSpace(fmt.Sprint(metadata["description"]))
	return sensitiveAutomationText(strings.Join([]string{target, name, description}, " "))
}

// prepareComputerTypeAuthorization inspects only an already-resolved native AX
// element before approval. It never triggers semantic/visual discovery merely to
// classify an input request. If classification cannot be established, the caller
// fails closed to a fresh approval rather than remembering permission for an
// unknown input field.
func (c *Controller) prepareComputerTypeAuthorization(ctx context.Context, args map[string]any, target string) bool {
	if c == nil || c.Computer == nil {
		return true
	}
	if native, _ := c.Computer.Capabilities["nativeAXBackend"].(bool); !native {
		return true
	}
	windowID := stringArg(args, "windowId")
	if windowID == "" || windowID == "screen:main" {
		return true
	}
	elementID := stringArg(args, "elementId")
	if strings.HasPrefix(elementID, "vision:") {
		return true
	}
	if elementID != "" {
		value, err := c.Computer.Call(ctx, "element_read", map[string]any{"windowId": windowID, "elementId": elementID})
		if err != nil {
			return true
		}
		metadata, _ := value.(map[string]any)
		return computerElementSensitive(metadata, target)
	}
	// A semantic target may resolve through a degraded accessibility tree and
	// Vision fallback. Do not inspect/capture extra UI before the input approval
	// merely to classify the target; unknown semantic typing fails closed instead.
	return true
}
