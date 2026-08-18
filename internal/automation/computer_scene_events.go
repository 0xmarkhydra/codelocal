package automation

import (
	"context"
	"fmt"
	"strings"
)

func (c *ComputerController) EventDrivenSceneSupported() bool {
	if c == nil {
		return false
	}
	enabled, _ := c.Capabilities["eventDrivenScene"].(bool)
	return enabled
}

func (c *ComputerController) SyncSceneEvents(ctx context.Context) error {
	if c == nil || !c.EventDrivenSceneSupported() {
		return nil
	}
	value, err := c.Call(ctx, "scene_events", map[string]any{})
	if err != nil {
		// Never trust a cache after the event channel fails. A subsequent read
		// must rebuild the scene instead of silently using stale accessibility data.
		c.MarkSceneDirty("", "scene-event-sync-failed")
		return err
	}
	root, ok := value.(map[string]any)
	if !ok {
		c.MarkSceneDirty("", "scene-event-sync-invalid")
		return fmt.Errorf("native scene event sync returned invalid payload")
	}
	events, _ := root["events"].([]any)
	for _, raw := range events {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		kind := strings.TrimSpace(fmt.Sprint(item["kind"]))
		windowID := strings.TrimSpace(fmt.Sprint(item["windowId"]))
		if windowID == "<nil>" {
			windowID = ""
		}
		elementID := strings.TrimSpace(fmt.Sprint(item["elementId"]))
		if elementID == "<nil>" {
			elementID = ""
		}
		c.NotifySceneEvent(ComputerSceneEvent{WindowID: windowID, Kind: kind, ElementID: elementID})
	}
	return nil
}
