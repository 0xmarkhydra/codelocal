package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var agentCursorProbe struct {
	sync.Mutex
	checkedAt time.Time
	supported bool
}

func probeMacAgentCursor() bool {
	agentCursorProbe.Lock()
	defer agentCursorProbe.Unlock()
	now := time.Now()
	if !agentCursorProbe.checkedAt.IsZero() && now.Sub(agentCursorProbe.checkedAt) < 30*time.Second {
		return agentCursorProbe.supported
	}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", `ObjC.import('Cocoa'); function run(){return String(Number($.NSScreen.screens.count)===1);}`)
	output, err := cmd.Output()
	agentCursorProbe.supported = err == nil && strings.EqualFold(strings.TrimSpace(string(output)), "true")
	agentCursorProbe.checkedAt = now
	return agentCursorProbe.supported
}

func (c *ComputerController) AgentCursorSupported() bool {
	if c == nil || c.Helper == "" || runtime.GOOS != "darwin" || !strings.Contains(strings.ToLower(c.Backend), "macos") {
		return false
	}
	uiTree, _ := c.Capabilities["uiTree"].(bool)
	return uiTree && probeMacAgentCursor()
}

// AgentCursor is internal runtime plumbing: callers must gate it with
// AgentCursorSupported. It renders visual-only feedback and never performs
// input, approval, or permission changes.
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
