package cloudserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
)

type dashboardExecutionState struct {
	workspace *gateway.WorkspaceView
	sessionID string
}

type dashboardExecutionStateKey struct{}

func dashboardWithExecutionState(r *http.Request, userID string, workspace *gateway.WorkspaceView) *http.Request {
	state := &dashboardExecutionState{workspace: workspace, sessionID: "dashboard-" + userID + "-" + cloud.RandomHex(8)}
	return r.WithContext(context.WithValue(r.Context(), dashboardExecutionStateKey{}, state))
}

func dashboardExecutionStateFromRequest(r *http.Request) *dashboardExecutionState {
	state, _ := r.Context().Value(dashboardExecutionStateKey{}).(*dashboardExecutionState)
	return state
}

func dashboardSetExecutionWorkspace(r *http.Request, workspace *gateway.WorkspaceView) {
	if state := dashboardExecutionStateFromRequest(r); state != nil {
		state.workspace = workspace
	}
}

var dashboardRuntimeChatTools = []map[string]any{
	dashboardRuntimeTool("list_project_files", "List files in the active CodeLocal project runtime.", map[string]any{
		"workspace": map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"path":      map[string]any{"type": "string", "description": "Optional workspace-relative directory."},
		"maxDepth":  map[string]any{"type": "integer"},
	}),
	dashboardRuntimeTool("read_project_file", "Read a file from the active CodeLocal project runtime. Use this before editing code.", map[string]any{
		"workspace": map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"path":      map[string]any{"type": "string"},
		"startLine": map[string]any{"type": "integer"},
		"endLine":   map[string]any{"type": "integer"},
	}, "path"),
	dashboardRuntimeTool("search_project_code", "Search text in the active CodeLocal project runtime.", map[string]any{
		"workspace":  map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"query":      map[string]any{"type": "string"},
		"path":       map[string]any{"type": "string"},
		"maxResults": map[string]any{"type": "integer"},
	}, "query"),
	dashboardRuntimeTool("edit_project_file", "Edit an existing project file by exact replacement. This executes on the user's CodeLocal runtime.", map[string]any{
		"workspace":    map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"path":         map[string]any{"type": "string"},
		"oldText":      map[string]any{"type": "string"},
		"newText":      map[string]any{"type": "string"},
		"replaceAll":   map[string]any{"type": "boolean"},
		"expectedHash": map[string]any{"type": "string"},
	}, "path", "oldText", "newText"),
	dashboardRuntimeTool("write_project_file", "Create or replace a project file on the user's CodeLocal runtime.", map[string]any{
		"workspace": map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"path":      map[string]any{"type": "string"},
		"content":   map[string]any{"type": "string"},
	}, "path", "content"),
	dashboardRuntimeTool("apply_project_patch", "Apply a unified Git patch to the active CodeLocal project runtime.", map[string]any{
		"workspace": map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"patch":     map[string]any{"type": "string"},
	}, "patch"),
	dashboardRuntimeTool("run_project_command", "Run a guarded shell command in the active CodeLocal project runtime. Use for tests, builds, linters and project commands.", map[string]any{
		"workspace": map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"command":   map[string]any{"type": "string"},
		"cwd":       map[string]any{"type": "string"},
		"timeoutMs": map[string]any{"type": "integer"},
	}, "command"),
	dashboardRuntimeTool("verify_project_changes", "Verify edits through the CodeLocal runtime and return diagnostics/diff checks.", map[string]any{
		"workspace": map[string]any{"type": "string", "description": "Optional workspace/project name or id."},
		"paths":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}),
}

func dashboardRuntimeTool(name, description string, properties map[string]any, required ...string) map[string]any {
	parameters := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		parameters["required"] = required
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": parameters}}
}

func dashboardRuntimeToolSpec(name string, args map[string]any) (runtimeTool string, sideEffect bool, forward map[string]any, ok bool) {
	forward = make(map[string]any, len(args))
	for key, value := range args {
		if key != "workspace" {
			forward[key] = value
		}
	}
	switch name {
	case "list_project_files":
		return "list_files", false, forward, true
	case "read_project_file":
		if _, hasStart := forward["startLine"]; hasStart {
			if _, hasEnd := forward["endLine"]; hasEnd {
				return "read_file_range", false, forward, true
			}
		}
		delete(forward, "startLine")
		delete(forward, "endLine")
		return "read_file", false, forward, true
	case "search_project_code":
		return "search_code", false, forward, true
	case "edit_project_file":
		return "edit_file", true, forward, true
	case "write_project_file":
		return "write_file", true, forward, true
	case "apply_project_patch":
		return "apply_patch", true, forward, true
	case "run_project_command":
		return "run_command", true, forward, true
	case "verify_project_changes":
		return "verify_changes", false, forward, true
	default:
		return "", false, nil, false
	}
}

func dashboardResolveRuntimeWorkspace(r *http.Request, s *Server, userID string, args map[string]any) (*gateway.WorkspaceView, error) {
	if s.Workspaces == nil {
		return nil, fmt.Errorf("workspace service unavailable")
	}
	if requested, _ := args["workspace"].(string); strings.TrimSpace(requested) != "" {
		catalog, err := s.Workspaces.Catalog(r.Context(), userID)
		if err != nil {
			return nil, err
		}
		workspace := dashboardChatFindWorkspace(catalog, requested)
		if workspace == nil {
			return nil, fmt.Errorf("workspace not found: %s", requested)
		}
		active, err := dashboardChatActivateWorkspace(r.Context(), s, userID, workspace)
		if err != nil {
			return nil, err
		}
		dashboardSetExecutionWorkspace(r, active)
		return active, nil
	}
	if state := dashboardExecutionStateFromRequest(r); state != nil && state.workspace != nil {
		return state.workspace, nil
	}
	catalog, err := s.Workspaces.Catalog(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	if current := dashboardChatCurrentWorkspace(catalog); current != nil {
		dashboardSetExecutionWorkspace(r, current)
		return current, nil
	}
	return nil, fmt.Errorf("no active project context; call get_workspace_detail first or provide workspace")
}

func execDashboardRuntimeTool(r *http.Request, s *Server, userID, name string, args map[string]any) (string, bool) {
	runtimeTool, sideEffect, forward, ok := dashboardRuntimeToolSpec(name, args)
	if !ok {
		return "", false
	}
	if s.Hub == nil {
		return `{"error":"runtime_gateway_unavailable"}`, true
	}
	workspace, err := dashboardResolveRuntimeWorkspace(r, s, userID, args)
	if err != nil {
		b, _ := json.Marshal(map[string]any{"error": "workspace_unavailable", "message": err.Error()})
		return string(b), true
	}
	state := dashboardExecutionStateFromRequest(r)
	sessionID := "dashboard-" + userID
	if state != nil && state.sessionID != "" {
		sessionID = state.sessionID
	}
	requestID := cloud.RandomHex(16)
	result, err := s.Hub.Call(r.Context(), userID, workspace.Key, sessionID, runtimeTool, forward, sideEffect, requestID)
	if err != nil && !sideEffect && s.Workspaces != nil {
		// Read-only runtime operations are safe to replay after a route loss. This
		// mirrors the MCP gateway rebind behavior while keeping mutations strictly
		// single-shot unless the runtime itself provides an approval/retry flow.
		if rebound, rebindErr := s.Workspaces.Activate(r.Context(), userID, workspace.Key); rebindErr == nil && rebound != nil {
			workspace = rebound
			dashboardSetExecutionWorkspace(r, rebound)
			result, err = s.Hub.Call(r.Context(), userID, workspace.Key, sessionID, runtimeTool, forward, sideEffect, requestID)
		}
	}
	if err != nil {
		b, _ := json.Marshal(map[string]any{"error": "runtime_call_failed", "message": err.Error()})
		return string(b), true
	}
	payload := map[string]any{"ok": result.OK, "workspace": workspace.WorkspaceName, "runtimeTool": runtimeTool, "result": result.Result, "metadata": result.Metadata}
	if !result.OK {
		payload["error"] = result.ErrorCode
		payload["message"] = result.Error
	}
	b, _ := json.Marshal(payload)
	return string(b), true
}
