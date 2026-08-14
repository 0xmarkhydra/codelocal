package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// AgentCursor asks the native helper to render a visual-only cursor. It is an
// internal UX operation, not an MCP capability: all approved input still flows
// through the normal computer operations and authorizer.
func (c *ComputerController) AgentCursor(ctx context.Context, args map[string]any) (any, error) {
	if c == nil || c.Helper == "" {
		return nil, errors.New("Computer Use helper unavailable")
	}
	request := map[string]any{
		"version":       1,
		"operation":     "cursor",
		"workspaceId":   c.WorkspaceID,
		"workspaceRoot": c.Root,
		"arguments":     args,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	response, err := c.callLocked(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("agent cursor helper failed: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "agent cursor operation failed"
		}
		return nil, errors.New(response.Error)
	}
	return response.Result, nil
}
