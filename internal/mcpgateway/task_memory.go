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
	}
	if publicTool == "verify" {
		patch.RecentChecks = []string{operation.OperationID}
		patch.ReplaceErrors = true
	}
	if publicTool == "terminal" {
		if command, _ := args["command"].(string); strings.TrimSpace(command) != "" {
			patch.RecentChecks = []string{command}
		}
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
	root["taskMemory"] = map[string]any{
		"task": state.Task,
		"branch": state.Branch,
		"touchedFiles": state.TouchedFiles,
		"recentChecks": state.RecentChecks,
		"recentErrors": state.RecentErrors,
		"lastAction": state.LastAction,
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
