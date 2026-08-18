package automation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func countComputerWindows(value any) (applicationWindows, fallbackWindows int) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			apps, fallbacks := countComputerWindows(item)
			applicationWindows += apps
			fallbackWindows += fallbacks
		}
	case map[string]any:
		windowID := strings.TrimSpace(fmt.Sprint(typed["windowId"]))
		if windowID != "" && windowID != "<nil>" {
			fallback, _ := typed["fallback"].(bool)
			if windowID == "screen:main" || fallback {
				return 0, 1
			}
			return 1, 0
		}
		for _, child := range typed {
			apps, fallbacks := countComputerWindows(child)
			applicationWindows += apps
			fallbackWindows += fallbacks
		}
	}
	return applicationWindows, fallbackWindows
}

func computerWindowEnumerationHealth(value any) map[string]any {
	applicationWindows, fallbackWindows := countComputerWindows(value)
	health := map[string]any{
		"applicationWindowCount": applicationWindows,
		"fallbackWindowCount":    fallbackWindows,
	}
	if applicationWindows > 0 {
		health["status"] = "healthy"
		return health
	}
	health["status"] = "degraded"
	if fallbackWindows > 0 {
		health["reason"] = "WINDOW_ENUMERATION_FALLBACK_ONLY"
	} else {
		health["reason"] = "WINDOW_ENUMERATION_EMPTY"
	}
	return health
}

func (c *ComputerController) Health(ctx context.Context) map[string]any {
	started := time.Now()
	windows, err := c.Windows(ctx, true)
	if err != nil {
		return map[string]any{
			"status":     "degraded",
			"reason":     "WINDOW_ENUMERATION_FAILED",
			"error":      err.Error(),
			"durationMs": time.Since(started).Milliseconds(),
		}
	}
	health := computerWindowEnumerationHealth(windows)
	health["durationMs"] = time.Since(started).Milliseconds()
	return health
}
