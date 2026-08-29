package cloudserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
)

type dashboardAccessChoice struct {
	Mode  string
	Label string
}

func dashboardRequestedAccessChoice(message string) (dashboardAccessChoice, bool) {
	normalized := dashboardChatNormalizeWorkspaceText(message)
	switch normalized {
	case "toàn quyền truy cập", "full access", "full":
		return dashboardAccessChoice{Mode: "full", Label: "Toàn quyền truy cập"}, true
	case "phê duyệt giúp tôi", "smart", "smart mode":
		return dashboardAccessChoice{Mode: "smart", Label: "Phê duyệt giúp tôi"}, true
	case "yêu cầu phê duyệt", "prompt", "prompt mode":
		return dashboardAccessChoice{Mode: "prompt", Label: "Yêu cầu phê duyệt"}, true
	default:
		return dashboardAccessChoice{}, false
	}
}

func dashboardAccessWorkspace(ctx context.Context, s *Server, userID string, current *gateway.WorkspaceView) (*gateway.WorkspaceView, error) {
	if current != nil {
		return current, nil
	}
	if s.Workspaces == nil {
		return nil, fmt.Errorf("workspace service unavailable")
	}
	catalog, err := s.Workspaces.Catalog(ctx, userID)
	if err != nil {
		return nil, err
	}
	workspace := dashboardChatCurrentWorkspace(catalog)
	if workspace == nil {
		return nil, fmt.Errorf("no active workspace to change access mode")
	}
	return dashboardChatActivateWorkspace(ctx, s, userID, workspace)
}

func dashboardApplyAccessChoice(ctx context.Context, s *Server, userID string, workspace *gateway.WorkspaceView, choice dashboardAccessChoice) (string, error) {
	if s.Hub == nil {
		return "", fmt.Errorf("runtime gateway unavailable")
	}
	if workspace == nil {
		return "", fmt.Errorf("workspace unavailable")
	}
	result, err := s.Hub.Call(ctx, userID, workspace.Key, "dashboard-access-"+userID, "approval_mode", map[string]any{"mode": choice.Mode}, true, cloud.RandomHex(16))
	if err != nil {
		return "", err
	}
	if !result.OK {
		if strings.TrimSpace(result.Error) != "" {
			return "", fmt.Errorf("%s", result.Error)
		}
		return "", fmt.Errorf("unable to update workspace access")
	}
	label := choice.Label
	if state, ok := result.Result.(map[string]any); ok {
		if value, _ := state["label"].(string); strings.TrimSpace(value) != "" {
			label = value
		}
	}
	return label, nil
}

func dashboardAccessResumeInstruction(choice dashboardAccessChoice, label string, workspace *gateway.WorkspaceView) map[string]any {
	workspaceName := "the current workspace"
	if workspace != nil && strings.TrimSpace(workspace.WorkspaceName) != "" {
		workspaceName = workspace.WorkspaceName
	}
	content := fmt.Sprintf("The user explicitly selected CodeLocal workspace access mode %q (%s) for %q, and CodeLocal has already applied it on the local runtime. Resume the immediately preceding blocked task now instead of merely acknowledging the access change. Do not ask the user to navigate elsewhere to approve the same action. If the blocked task is a Git push and the branch is behind/diverged, inspect Git state, fetch/rebase onto the remote branch, then retry the push. Stop only for a real rebase conflict, hard security block, offline runtime, or another explicit approval requirement that still remains after the mode change.", label, choice.Mode, workspaceName)
	return map[string]any{"role": "system", "content": content}
}

func dashboardAccessEvent(choice dashboardAccessChoice, label string, workspace *gateway.WorkspaceView) map[string]any {
	payload := map[string]any{"mode": choice.Mode, "label": label}
	if workspace != nil {
		payload["workspaceId"] = workspace.WorkspaceID
		payload["workspaceName"] = workspace.WorkspaceName
	}
	return payload
}

func dashboardMarshalAccessError(err error) string {
	payload, _ := json.Marshal(map[string]any{"error": "access_mode_failed", "message": err.Error()})
	return string(payload)
}
