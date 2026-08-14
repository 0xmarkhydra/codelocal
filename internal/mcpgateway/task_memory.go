package mcpgateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var workingMemory = taskstate.New(512)

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
		patch.Task, _ = args["taskHint"].(string)
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

func attachTaskMemory(result *mcp.CallToolResult, state taskstate.State) {
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
	result.StructuredContent = root
}

func (s *Service) callOperationRemembering(ctx context.Context, userID, publicTool string, operation operationInvocation, args map[string]any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session := sessionID(req)
	workspaceKey := memoryWorkspaceKey(s, userID, session, args)
	result, err := s.callOperation(ctx, userID, publicTool, operation, args, req)
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if workspaceKey == "" {
		return result, err
	}
	state := workingMemory.Update(userID, session, workspaceKey, taskPatchForOperation(publicTool, operation, args, result))
	if publicTool == "context" && err == nil && result != nil && !result.IsError {
		attachTaskMemory(result, state)
	}
	return result, err
}
