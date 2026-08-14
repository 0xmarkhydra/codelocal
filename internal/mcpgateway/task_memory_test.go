package mcpgateway

import (
	"testing"

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

func TestAttachTaskMemoryProjectsOperationalState(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"results": []any{}}}
	attachTaskMemory(result, taskstate.State{
		Task:         "Fix login",
		Branch:       "feat/login",
		TouchedFiles: []string{"a.go"},
		RecentChecks: []string{"verify.changes"},
		LastAction:   "verify.changes",
	})
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected map structured content, got %#v", result.StructuredContent)
	}
	memory, ok := root["taskMemory"].(map[string]any)
	if !ok || memory["task"] != "Fix login" || memory["branch"] != "feat/login" {
		t.Fatalf("unexpected task memory: %#v", root["taskMemory"])
	}
}
