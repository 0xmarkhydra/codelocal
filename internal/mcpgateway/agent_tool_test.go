package mcpgateway

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mustRuntimeOperation(t *testing.T, tool string) operationInvocation {
	t.Helper()
	operation, err := operationForRuntimeTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func TestAutonomousStepPolicyRequiresHashSafeMutation(t *testing.T) {
	plan := orchestration.AgentPlan{Route: orchestration.Decision{Primary: orchestration.LaneCode, Confidence: .9}}
	operation := mustRuntimeOperation(t, "edit_file")
	if decision := autonomousStepPolicy(operation, map[string]any{"path": "x.go", "oldText": "a", "newText": "b"}, plan); decision.Allowed {
		t.Fatalf("unprotected edit should not run autonomously: %#v", decision)
	}
	if decision := autonomousStepPolicy(operation, map[string]any{"path": "x.go", "oldText": "a", "newText": "b", "expectedHash": "abc"}, plan); !decision.Allowed {
		t.Fatalf("hash-safe workspace edit should be allowed: %#v", decision)
	}
}

func TestAutonomousStepPolicyBlocksApprovalOpenWorldAndGitWrites(t *testing.T) {
	plan := orchestration.AgentPlan{Route: orchestration.Decision{Primary: orchestration.LaneCode, Confidence: .95}}
	commit := mustRuntimeOperation(t, "git_commit")
	if decision := autonomousStepPolicy(commit, map[string]any{"message": "x"}, plan); decision.Allowed {
		t.Fatalf("git commit must never be autonomous: %#v", decision)
	}
	open := mustRuntimeOperation(t, "browser_open")
	if decision := autonomousStepPolicy(open, map[string]any{"url": "https://example.com"}, plan); decision.Allowed {
		t.Fatalf("open-world navigation must not be autonomous: %#v", decision)
	}
	read := mustRuntimeOperation(t, "read_file")
	if decision := autonomousStepPolicy(read, map[string]any{"path": "x.go", "approvalToken": "token"}, plan); decision.Allowed {
		t.Fatalf("bounded executor must never consume approval tokens: %#v", decision)
	}
}

func TestSafeAutonomousVerificationCommandRejectsShellComposition(t *testing.T) {
	for _, command := range []string{"go test ./internal/foo", "go vet ./...", "git diff --check", "npm run typecheck"} {
		if !safeAutonomousVerificationCommand(command) {
			t.Fatalf("expected safe verification command %q", command)
		}
	}
	for _, command := range []string{"go test ./... && rm -rf tmp", "curl https://example.com", "npm publish", "go test ./... | cat", "go testevil"} {
		if safeAutonomousVerificationCommand(command) {
			t.Fatalf("unsafe command accepted for autonomous execution: %q", command)
		}
	}
}

func TestAutonomousStepPolicyAllowsRecognizedVerificationCommand(t *testing.T) {
	plan := orchestration.AgentPlan{Route: orchestration.Decision{Primary: orchestration.LaneCode, Confidence: .95}}
	operation := mustRuntimeOperation(t, "run_command")
	decision := autonomousStepPolicy(operation, map[string]any{"command": "go test ./internal/foo"}, plan)
	if !decision.Allowed {
		t.Fatalf("recognized verification command should remain eligible despite terminal open-world classification: %#v", decision)
	}
}

func TestResultNeedsAgentHaltForRunningAndFailedVerification(t *testing.T) {
	if halt, reason := resultNeedsAgentHalt(&mcp.CallToolResult{StructuredContent: map[string]any{"running": true, "processId": "p1"}}); !halt || reason == "" {
		t.Fatalf("running verification must halt bounded execution until polled: halt=%v reason=%q", halt, reason)
	}
	if halt, reason := resultNeedsAgentHalt(&mcp.CallToolResult{StructuredContent: map[string]any{"running": false, "exitCode": 2}}); !halt || reason == "" {
		t.Fatalf("non-zero verification must halt bounded execution: halt=%v reason=%q", halt, reason)
	}
	if halt, reason := resultNeedsAgentHalt(&mcp.CallToolResult{StructuredContent: map[string]any{"status": "approval_required", "reason": "review required"}}); !halt || reason != "review required" {
		t.Fatalf("approval-required result must return control without auto-confirming: halt=%v reason=%q", halt, reason)
	}
}

func TestVerificationCheckOutcomeWaitsForProcessCompletion(t *testing.T) {
	operation := mustRuntimeOperation(t, "run_command")
	args := map[string]any{"command": "go test ./internal/foo"}
	key, completed, success, _ := verificationCheckOutcome(operation, args, &mcp.CallToolResult{StructuredContent: map[string]any{
		"processId": "p1", "running": true, "status": "running",
	}})
	if key != "test" || completed || success {
		t.Fatalf("running verification must not count as pass: key=%q completed=%v success=%v", key, completed, success)
	}

	key, completed, success, failure := verificationCheckOutcome(operation, args, &mcp.CallToolResult{StructuredContent: map[string]any{
		"processId": "p1", "running": false, "status": "exited", "exitCode": 1,
	}})
	if key != "test" || !completed || success || failure == "" {
		t.Fatalf("failed process must become failed verification evidence: key=%q completed=%v success=%v failure=%q", key, completed, success, failure)
	}
}

func TestProcessPollCanCompleteVerificationEvidence(t *testing.T) {
	operation := mustRuntimeOperation(t, "exec_poll")
	result := &mcp.CallToolResult{StructuredContent: map[string]any{
		"command": "go test ./internal/foo", "running": false, "status": "exited", "exitCode": 0,
	}}
	patch := agentPatchForOperation(operation, map[string]any{"processId": "p1"}, result, taskstate.State{AgentPhase: "verify"})
	if len(patch.PassedChecks) != 1 || patch.PassedChecks[0] != "test" {
		t.Fatalf("completed poll should contribute verification evidence: %#v", patch)
	}
	if !shouldRefreshQuality(operation, map[string]any{"processId": "p1"}, result) {
		t.Fatal("completed verification poll should refresh quality")
	}
}

func TestApprovalRequiredVerificationNeverConsumesRecoveryBudget(t *testing.T) {
	operation := mustRuntimeOperation(t, "run_command")
	result := &mcp.CallToolResult{StructuredContent: map[string]any{
		"status": "approval_required", "reason": "workspace code execution",
	}}
	patch := agentPatchForOperation(operation, map[string]any{"command": "go test ./..."}, result, taskstate.State{AgentPhase: "verify"})
	if patch.AgentPhase != "recover" || patch.RecoveryAttempts == nil || *patch.RecoveryAttempts != 0 {
		t.Fatalf("approval-required verification must stop without automatic retry: %#v", patch)
	}
}

func TestAgentToolIsPresentAndHasSpecialExecutor(t *testing.T) {
	definitions := compactDefinitionsByName()
	agent, ok := definitions["agent"]
	if !ok || agent.Execute == nil || agent.Resolve != nil {
		t.Fatalf("agent tool must use special bounded executor: %#v", agent)
	}
}
