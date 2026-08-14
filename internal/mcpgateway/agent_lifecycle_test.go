package mcpgateway

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func applyAgentEvent(store *taskstate.Store, user, session, workspace, publicTool string, operation operationInvocation, args map[string]any, result *mcp.CallToolResult) taskstate.State {
	state := store.Update(user, session, workspace, taskPatchForOperation(publicTool, operation, args, result))
	agentPatch := agentPatchForOperation(operation, args, result, state)
	state = store.Update(user, session, workspace, agentPatch)
	if operation.OperationID == "verify.changes" || (operation.OperationID == "terminal.run" && len(agentPatch.PassedChecks) > 0) {
		state = store.Update(user, session, workspace, qualityPatchForState(state))
	}
	return state
}

func TestAgentLifecycleContextEditVerifyChecksFinalize(t *testing.T) {
	store := taskstate.New(16)
	const user, session, workspace = "u", "s", "w"

	state := applyAgentEvent(store, user, session, workspace, "context", operationInvocation{OperationID: "context.task"}, map[string]any{
		"taskHint": "fix planner regression",
	}, &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}})
	if state.AgentPhase != "inspect" {
		t.Fatalf("context should enter inspect phase: %#v", state)
	}

	state = applyAgentEvent(store, user, session, workspace, "edit", operationInvocation{OperationID: "edit.apply"}, map[string]any{
		"path": "internal/orchestration/planner.go",
	}, &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}})
	if state.AgentPhase != "verify" || state.AgentIteration != 1 || len(state.TouchedFiles) != 1 {
		t.Fatalf("edit should enter verification with touched-file evidence: %#v", state)
	}

	diffCheckID := orchestration.CheckID("git diff --check")
	testCheckID := orchestration.CheckID("go test ./internal/orchestration")
	verifyResult := &mcp.CallToolResult{StructuredContent: map[string]any{
		"diagnosticRegression": 0,
		"verificationScope":    []string{"internal/orchestration/planner.go"},
		"gitDiff":              "diff --git a/internal/orchestration/planner.go b/internal/orchestration/planner.go",
		"verificationPlan": map[string]any{"checks": []any{
			map[string]any{"key": diffCheckID, "required": true},
			map[string]any{"key": testCheckID, "required": true},
		}},
	}}
	state = applyAgentEvent(store, user, session, workspace, "verify", operationInvocation{OperationID: "verify.changes"}, nil, verifyResult)
	if !state.VerificationSeen || !state.DiffObserved || state.QualityStatus != "verifying" {
		t.Fatalf("fresh verify evidence should keep task verifying until required checks pass: %#v", state)
	}
	if len(state.RequiredChecks) != 2 {
		t.Fatalf("verify should refresh required checks from actual diff: %#v", state.RequiredChecks)
	}

	state = applyAgentEvent(store, user, session, workspace, "terminal", operationInvocation{OperationID: "terminal.run"}, map[string]any{
		"command": "git diff --check",
	}, &mcp.CallToolResult{StructuredContent: map[string]any{"exitCode": 0}})
	if state.AgentPhase != "verify" || state.QualityStatus != "verifying" {
		t.Fatalf("one required check should not finalize task: %#v", state)
	}

	state = applyAgentEvent(store, user, session, workspace, "terminal", operationInvocation{OperationID: "terminal.run"}, map[string]any{
		"command": "go test ./internal/orchestration",
	}, &mcp.CallToolResult{StructuredContent: map[string]any{"exitCode": 0}})
	if state.AgentPhase != "finalize" || state.QualityStatus != "ready" {
		t.Fatalf("all required evidence should move task to finalize: %#v", state)
	}
	if len(state.PassedChecks) != 2 {
		t.Fatalf("expected both safe verification categories to persist: %#v", state.PassedChecks)
	}

	result := &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	attachAgentLoop(result, state)
	loop := result.StructuredContent.(map[string]any)["agentLoop"].(map[string]any)
	if allowed, _ := loop["completionAllowed"].(bool); !allowed {
		t.Fatalf("completion should only be allowed after quality gate: %#v", loop)
	}
}

func TestAgentLifecycleFailureRecoveryIsBoundedAndEvidenceDriven(t *testing.T) {
	store := taskstate.New(16)
	const user, session, workspace = "u", "s", "w"
	store.Update(user, session, workspace, taskstate.Patch{Task: "fix stale desktop flow", AgentPhase: "inspect"})

	failure := &mcp.CallToolResult{IsError: true, StructuredContent: map[string]any{"error": "stale element after navigation"}}
	state := applyAgentEvent(store, user, session, workspace, "computer", operationInvocation{OperationID: "computer.click"}, map[string]any{"target": "Continue"}, failure)
	if state.AgentPhase != "recover" || state.RecoveryAttempts != 1 {
		t.Fatalf("retryable stale UI failure should enter bounded recovery: %#v", state)
	}

	state = applyAgentEvent(store, user, session, workspace, "computer", operationInvocation{OperationID: "computer.click"}, map[string]any{"target": "Continue"}, failure)
	state = applyAgentEvent(store, user, session, workspace, "computer", operationInvocation{OperationID: "computer.click"}, map[string]any{"target": "Continue"}, failure)
	if state.RecoveryAttempts != 2 {
		t.Fatalf("recovery budget must cap at two attempts: %#v", state)
	}

	permission := &mcp.CallToolResult{IsError: true, StructuredContent: map[string]any{"error": "approval required"}}
	freshStore := taskstate.New(16)
	freshStore.Update(user, session, workspace, taskstate.Patch{Task: "desktop action", AgentPhase: "inspect"})
	blocked := applyAgentEvent(freshStore, user, session, workspace, "computer", operationInvocation{OperationID: "computer.click"}, map[string]any{"target": "Continue"}, permission)
	if blocked.AgentPhase != "recover" || blocked.RecoveryAttempts != 0 {
		t.Fatalf("permission failure must not consume an automatic retry: %#v", blocked)
	}
}
