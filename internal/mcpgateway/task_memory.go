package mcpgateway

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	workingMemory      = taskstate.New(512)
	memorySecretAssign = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|token|secret|password|passwd)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	memoryBearer       = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func sanitizeTaskMemoryText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = memorySecretAssign.ReplaceAllString(value, "$1=[REDACTED]")
	value = memoryBearer.ReplaceAllString(value, "Bearer [REDACTED]")
	runes := []rune(value)
	if len(runes) > 360 {
		value = string(runes[:360]) + "…"
	}
	return value
}

func stringSliceArg(args map[string]any, key string) []string {
	value, ok := args[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func structuredFilePaths(args map[string]any, key string) []string {
	items, ok := args[key].([]any)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path, _ := entry["path"].(string)
		if path = strings.TrimSpace(path); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func taskPatchForOperation(publicTool string, operation operationInvocation, args map[string]any, result *mcp.CallToolResult) taskstate.Patch {
	patch := taskstate.Patch{LastAction: operation.OperationID}
	if publicTool == "context" {
		if task, _ := args["taskHint"].(string); task != "" {
			patch.Task = sanitizeTaskMemoryText(task)
		}
	}
	if publicTool == "edit" {
		if path, _ := args["path"].(string); strings.TrimSpace(path) != "" {
			patch.TouchedFiles = append(patch.TouchedFiles, path)
		}
		patch.TouchedFiles = append(patch.TouchedFiles, stringSliceArg(args, "paths")...)
		patch.TouchedFiles = append(patch.TouchedFiles, structuredFilePaths(args, "files")...)
	}
	if publicTool == "git" {
		if branch, _ := args["branch"].(string); strings.TrimSpace(branch) != "" {
			patch.Branch = branch
		}
	}
	if publicTool == "verify" || publicTool == "terminal" {
		// Never persist raw terminal commands: they may contain credentials,
		// tokens, private paths or other sensitive arguments. The stable
		// operation identity is enough for continuation/recovery hints.
		patch.RecentChecks = []string{operation.OperationID}
	}
	if publicTool == "verify" && result != nil && !result.IsError {
		patch.ReplaceErrors = true
	}
	if result != nil && result.IsError {
		patch.RecentErrors = []string{operation.OperationID + " failed"}
	}
	return patch
}

func memoryWorkspaceKey(s *Service, userID, session string, args map[string]any) string {
	if explicit, _ := args["workspaceKey"].(string); strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	return strings.TrimSpace(s.route(userID, session))
}

func executionCapabilities(workspace *gateway.WorkspaceView) orchestration.Capabilities {
	if workspace == nil {
		return orchestration.Capabilities{}
	}
	filesystem := workspace.ProtocolVersion <= 1 || capabilityBool(workspace.Capabilities, "filesystem")
	automation := nestedMap(workspace.Capabilities["automation"])
	browser := nestedMap(automation["browser"])
	computer := nestedMap(automation["computer"])
	return orchestration.Capabilities{
		Filesystem: filesystem,
		LSP:        filesystem,
		Shell:      capabilityBool(workspace.Capabilities, "shell"),
		Browser:    capabilityFlag(browser, "available"),
		Computer:   capabilityFlag(computer, "available"),
	}
}

func attachTaskContext(result *mcp.CallToolResult, state taskstate.State, decision orchestration.Decision) {
	if result == nil {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	// StructuredContent is intentionally the only projection path. Duplicating
	// the same memory into TextContent would increase model tokens on every
	// context call while modern MCP clients already consume structured content.
	root["taskMemory"] = map[string]any{
		"task":         state.Task,
		"branch":       state.Branch,
		"touchedFiles": state.TouchedFiles,
		"recentChecks": state.RecentChecks,
		"recentErrors": state.RecentErrors,
		"lastAction":   state.LastAction,
	}
	root["routeHint"] = decision
	result.StructuredContent = root
}

func attachRecoveryHint(result *mcp.CallToolResult) {
	if result == nil || !result.IsError {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	message, _ := root["error"].(string)
	if strings.TrimSpace(message) == "" {
		return
	}
	root["recovery"] = orchestration.ClassifyFailure(message)
	result.StructuredContent = root
}

func (s *Service) callOperationRemembering(ctx context.Context, userID, publicTool string, operation operationInvocation, args map[string]any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session := sessionID(req)
	workspaceKey := memoryWorkspaceKey(s, userID, session, args)
	result, err := s.callOperation(ctx, userID, publicTool, operation, args, req)
	attachRecoveryHint(result)
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if workspaceKey == "" {
		return result, err
	}
	state := workingMemory.Update(userID, session, workspaceKey, taskPatchForOperation(publicTool, operation, args, result))
	if publicTool == "context" && err == nil && result != nil && !result.IsError {
		decision := orchestration.Decision{Primary: orchestration.LaneNone, Reason: "workspace capabilities unavailable"}
		if workspace, activateErr := s.Workspaces.Activate(ctx, userID, workspaceKey); activateErr == nil {
			decision = orchestration.Route(state.Task, executionCapabilities(workspace))
		}
		attachTaskContext(result, state, decision)
	}
	return result, err
}
