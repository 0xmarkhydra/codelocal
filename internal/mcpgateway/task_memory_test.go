package mcpgateway

import "testing"

func TestTaskPatchTracksContextAndTouchedFile(t *testing.T) {
	contextOp := operationInvocation{OperationID: "context.task"}
	patch := taskPatchForOperation("context", contextOp, map[string]any{"taskHint": "repair login flow"}, nil)
	if patch.Task != "repair login flow" || patch.LastAction != "context.task" {
		t.Fatalf("unexpected context patch: %#v", patch)
	}

	editOp := operationInvocation{OperationID: "edit.replace"}
	patch = taskPatchForOperation("edit", editOp, map[string]any{"path": "src/login.go"}, nil)
	if len(patch.TouchedFiles) != 1 || patch.TouchedFiles[0] != "src/login.go" {
		t.Fatalf("unexpected edit patch: %#v", patch)
	}
}
