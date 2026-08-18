package automation

import "context"

func (c *ComputerController) NativeBatchSupported() bool {
	if c == nil {
		return false
	}
	enabled, _ := c.Capabilities["batchActions"].(bool)
	return enabled
}

func (c *ComputerController) SemanticBatch(ctx context.Context, windowID string, steps []computerSequenceStep) (any, error) {
	rawSteps := make([]any, 0, len(steps))
	for _, step := range steps {
		entry := map[string]any{"operation": step.Operation, "target": step.Target}
		if step.Operation == "type" {
			entry["text"] = step.Text
		}
		rawSteps = append(rawSteps, entry)
	}
	value, err := c.Call(ctx, "semantic_batch", map[string]any{"windowId": windowID, "steps": rawSteps})
	if err == nil {
		lastElementID := ""
		if root, ok := value.(map[string]any); ok {
			if results, ok := root["results"].([]any); ok && len(results) > 0 {
				lastElementID = resolvedElementID(results[len(results)-1])
			}
		}
		c.NotifySceneEvent(ComputerSceneEvent{WindowID: windowID, Kind: "semantic-batch", ElementID: lastElementID})
	}
	return value, err
}
