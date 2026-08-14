package mcpgateway

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTaskPatchTracksContextAndEditPaths(t *testing.T) {
	contextOp := operationInvocation{OperationID: "context.task"}
	patch := taskPatchForOperation("context", contextOp, map[string]any{"taskHint": "Fix login"}, nil)
	if patch.Task != "Fix login" || patch.LastAction != "context.task" {
		t.Fatalf("unexpected context patch: %#v", patch)
	}

	editOp := operationInvocation{OperationID: "edit.apply"}
	patch = taskPatchForOperation("edit", editOp, map[string]any{
		"path":  "a.go",
		"paths": []any{"b.go", "c.go"},
		"files": []any{map[string]any{"path": "d.go"}},
	}, nil)
	if len(patch.TouchedFiles) != 4 {
		t.Fatalf("expected all touched paths, got %#v", patch.TouchedFiles)
	}
}

func TestTaskPatchDoesNotPersistRawTerminalCommand(t *testing.T) {
	op := operationInvocation{OperationID: "terminal.run"}
	patch := taskPatchForOperation("terminal", op, map[string]any{
		"command": "deploy --token super-secret-value",
	}, nil)
	if len(patch.RecentChecks) != 1 || patch.RecentChecks[0] != "terminal.run" {
		t.Fatalf("expected only stable operation identity, got %#v", patch.RecentChecks)
	}
}

func TestExecutionCapabilitiesUseAdvertisedAutomation(t *testing.T) {
	workspace := &gateway.WorkspaceView{ProtocolVersion: 3, Capabilities: map[string]any{
		"filesystem": true,
		"shell":      true,
		"automation": map[string]any{
			"browser":  map[string]any{"available": true},
			"computer": map[string]any{"available": false},
		},
	}}
	caps := executionCapabilities(workspace)
	if !caps.Filesystem || !caps.LSP || !caps.Shell || !caps.Browser || caps.Computer {
		t.Fatalf("unexpected execution capabilities: %#v", caps)
	}
}

func TestAttachTaskContextProjectsMemoryAndRoute(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"results": []any{}}}
	attachTaskContext(result, taskstate.State{
		Task:         "Fix login",
		Branch:       "feat/login",
		TouchedFiles: []string{"a.go"},
		RecentChecks: []string{"verify.changes"},
		LastAction:   "verify.changes",
	}, orchestration.Decision{Primary: orchestration.LaneCode, Reason: "test"})
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected map structured content, got %#v", result.StructuredContent)
	}
	memory, ok := root["taskMemory"].(map[string]any)
	if !ok || memory["task"] != "Fix login" || memory["branch"] != "feat/login" {
		t.Fatalf("unexpected task memory: %#v", root["taskMemory"])
	}
	decision, ok := root["routeHint"].(orchestration.Decision)
	if !ok || decision.Primary != orchestration.LaneCode {
		t.Fatalf("unexpected route hint: %#v", root["routeHint"])
	}
}

func TestAttachRecoveryHintIsConservative(t *testing.T) {
	result := &mcp.CallToolResult{IsError: true, StructuredContent: map[string]any{"error": "approval required for desktop action"}}
	attachRecoveryHint(result)
	root := result.StructuredContent.(map[string]any)
	advice, ok := root["recovery"].(orchestration.RecoveryAdvice)
	if !ok || advice.Kind != orchestration.FailurePermission || advice.Retryable {
		t.Fatalf("unexpected recovery advice: %#v", root["recovery"])
	}
}
