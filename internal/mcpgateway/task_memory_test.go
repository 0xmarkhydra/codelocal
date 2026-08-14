package mcpgateway

import (
	"strings"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
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

func TestTaskHintRedactsSecretLikeAssignments(t *testing.T) {
	op := operationInvocation{OperationID: "context.task"}
	patch := taskPatchForOperation("context", op, map[string]any{
		"taskHint": "Fix deploy api_key=super-secret-value then retry with Bearer abc.def.ghi",
	}, nil)
	if strings.Contains(patch.Task, "super-secret-value") || strings.Contains(patch.Task, "abc.def.ghi") {
		t.Fatalf("secret-like task content leaked into memory: %q", patch.Task)
	}
	if !strings.Contains(patch.Task, "[REDACTED]") {
		t.Fatalf("expected visible redaction marker, got %q", patch.Task)
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

func TestAttachTaskContextProjectsMemoryRouteAndAgentPlan(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"results": []any{}}}
	plan := orchestration.AgentPlan{
		Version:    1,
		Route:      orchestration.Decision{Primary: orchestration.LaneCode, Reason: "test"},
		Phase:      "finalize",
		NextAction: "finalize result",
		Quality:    orchestration.QualityEvaluation{Score: 94, Status: "ready"},
	}
	attachTaskContext(result, taskstate.State{
		Task:             "Fix login",
		Branch:           "feat/login",
		TouchedFiles:     []string{"a.go"},
		RecentChecks:     []string{"verify.changes"},
		LastAction:       "verify.changes",
		AgentPhase:       "finalize",
		AgentIteration:   2,
		PassedChecks:     []string{"test"},
		QualityScore:     94,
		QualityStatus:    "ready",
		VerificationSeen: true,
	}, plan)
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected map structured content, got %#v", result.StructuredContent)
	}
	memory, ok := root["taskMemory"].(map[string]any)
	if !ok || memory["task"] != "Fix login" || memory["branch"] != "feat/login" || memory["agentPhase"] != "finalize" {
		t.Fatalf("unexpected task memory: %#v", root["taskMemory"])
	}
	decision, ok := root["routeHint"].(orchestration.Decision)
	if !ok || decision.Primary != orchestration.LaneCode {
		t.Fatalf("unexpected route hint: %#v", root["routeHint"])
	}
	attachedPlan, ok := root["agentPlan"].(orchestration.AgentPlan)
	if !ok || attachedPlan.Quality.Status != "ready" || attachedPlan.Phase != "finalize" {
		t.Fatalf("unexpected agent plan: %#v", root["agentPlan"])
	}
}

func TestAgentPatchPersistsOnlyVerificationCheckCategory(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	patch := agentPatchForOperation(operationInvocation{OperationID: "terminal.run"}, map[string]any{
		"command": "go test ./internal/foo --token super-secret-value",
	}, result, taskstate.State{AgentPhase: "verify"})
	if len(patch.PassedChecks) != 1 || patch.PassedChecks[0] != "test" {
		t.Fatalf("expected safe check category only, got %#v", patch.PassedChecks)
	}
	if strings.Contains(strings.Join(patch.PassedChecks, " "), "secret") {
		t.Fatalf("terminal command leaked into durable agent state: %#v", patch.PassedChecks)
	}
}

func TestAgentPatchCapturesVerifyEvidence(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{
		"diagnosticRegression": 0,
		"verificationScope":    []string{"internal/foo/foo.go"},
		"gitDiff":              "diff --git a/internal/foo/foo.go b/internal/foo/foo.go",
		"verificationPlan": map[string]any{"checks": []any{
			map[string]any{"key": "diff-check", "required": true},
			map[string]any{"key": "test", "required": true},
			map[string]any{"key": "lint", "required": false},
		}},
	}}
	patch := agentPatchForOperation(operationInvocation{OperationID: "verify.changes"}, nil, result, taskstate.State{AgentPhase: "verify"})
	if patch.VerificationSeen == nil || !*patch.VerificationSeen || patch.DiagnosticRegression == nil || *patch.DiagnosticRegression != 0 {
		t.Fatalf("verify evidence missing: %#v", patch)
	}
	if patch.DiffObserved == nil || !*patch.DiffObserved {
		t.Fatalf("diff evidence missing: %#v", patch)
	}
	if !patch.ReplaceRequiredChecks || len(patch.RequiredChecks) != 2 || patch.RequiredChecks[0] != "diff-check" || patch.RequiredChecks[1] != "test" {
		t.Fatalf("verify result did not refresh required checks: %#v", patch)
	}
}

func TestCarryTaskStatePatchPreservesAgentState(t *testing.T) {
	state := taskstate.State{
		Task: "Fix planner", Branch: "feat/planner", TouchedFiles: []string{"planner.go"},
		AgentPhase: "verify", AgentIteration: 3, RecoveryAttempts: 1,
		PassedChecks: []string{"test", "diff-check"}, VerificationSeen: true,
		QualityScore: 90, QualityStatus: "ready",
	}
	patch := carryTaskStatePatch(state)
	if patch.AgentIteration == nil || *patch.AgentIteration != 3 || patch.RecoveryAttempts == nil || *patch.RecoveryAttempts != 1 {
		t.Fatalf("agent counters not carried: %#v", patch)
	}
	if !patch.ReplacePassedChecks || len(patch.PassedChecks) != 2 || patch.QualityStatus != "ready" {
		t.Fatalf("agent evidence not carried: %#v", patch)
	}
}

func TestAttachAgentLoopProjectsCompactState(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	attachAgentLoop(result, taskstate.State{
		Task: "Fix planner", AgentPhase: "verify", AgentIteration: 2, RecoveryAttempts: 0,
		NextAction: "run tests", PassedChecks: []string{"diff-check"}, QualityScore: 70, QualityStatus: "verifying",
	})
	root := result.StructuredContent.(map[string]any)
	loop, ok := root["agentLoop"].(map[string]any)
	if !ok || loop["phase"] != "verify" || loop["nextAction"] != "run tests" {
		t.Fatalf("unexpected agent loop projection: %#v", root["agentLoop"])
	}
}

func TestQualityPatchPromotesVerifiedTaskToFinalize(t *testing.T) {
	patch := qualityPatchForState(taskstate.State{
		TouchedFiles:     []string{"internal/foo/foo.go"},
		AgentPhase:       "verify",
		RequiredChecks:   []string{"diff-check", "test"},
		PassedChecks:     []string{"diff-check", "test"},
		VerificationSeen: true,
		DiffObserved:     true,
	})
	if patch.QualityStatus != "ready" || patch.AgentPhase != "finalize" {
		t.Fatalf("verified task should finalize: %#v", patch)
	}
}

func TestQualityPatchDoesNotDeclareReadyWithoutVerificationPlan(t *testing.T) {
	patch := qualityPatchForState(taskstate.State{
		TouchedFiles:     []string{"internal/foo/foo.go"},
		AgentPhase:       "verify",
		VerificationSeen: true,
		DiffObserved:     true,
	})
	if patch.QualityStatus == "ready" || patch.AgentPhase == "finalize" {
		t.Fatalf("task without required-check plan must not finalize: %#v", patch)
	}
}

func TestAgentCheckpointRestoresProgressButInvalidatesCompletionEvidence(t *testing.T) {
	now := time.Now()
	checkpointState := taskstate.State{
		Task: "Fix planner", AgentPhase: "finalize", AgentIteration: 4, RecoveryAttempts: 0,
		LastOutcome: "succeeded", TouchedFiles: []string{"internal/orchestration/planner.go"},
		PassedChecks: []string{"test", "diff-check"}, VerificationSeen: true,
		QualityScore: 100, QualityStatus: "ready",
	}
	record := longmemory.Record{
		CreatedAt: now.UnixMilli(), Branch: "feat/planner",
		Files:   append([]string(nil), checkpointState.TouchedFiles...),
		Symbols: agentCheckpointSymbols(checkpointState),
	}
	patch, ok := agentCheckpointPatch(taskstate.State{Task: "Fix planner", AgentPhase: "inspect"}, []longmemory.Record{record}, now)
	if !ok {
		t.Fatal("expected matching checkpoint to restore")
	}
	if patch.AgentPhase != "verify" || patch.AgentIteration == nil || *patch.AgentIteration != 4 {
		t.Fatalf("checkpoint should resume at fresh verification: %#v", patch)
	}
	if !patch.ReplacePassedChecks || len(patch.PassedChecks) != 0 || patch.VerificationSeen == nil || *patch.VerificationSeen {
		t.Fatalf("stale completion evidence must not survive runtime restore: %#v", patch)
	}
	if patch.QualityScore == nil || *patch.QualityScore != 0 || patch.QualityStatus == "ready" {
		t.Fatalf("restored checkpoint must re-earn quality: %#v", patch)
	}
}

func TestAgentCheckpointRequiresExactTaskFingerprintAndFreshRecord(t *testing.T) {
	now := time.Now()
	state := taskstate.State{Task: "Task A", AgentPhase: "verify", AgentIteration: 2}
	record := longmemory.Record{CreatedAt: now.UnixMilli(), Symbols: agentCheckpointSymbols(state)}
	if _, ok := agentCheckpointPatch(taskstate.State{Task: "Task B"}, []longmemory.Record{record}, now); ok {
		t.Fatal("semantic recall from another task must not restore agent state")
	}
	record.CreatedAt = now.Add(-8 * 24 * time.Hour).UnixMilli()
	if _, ok := agentCheckpointPatch(taskstate.State{Task: "Task A"}, []longmemory.Record{record}, now); ok {
		t.Fatal("stale checkpoint must not restore agent state")
	}
}

func TestAgentCheckpointSymbolsDoNotPersistRawTaskOrVerificationClaims(t *testing.T) {
	state := taskstate.State{
		Task: "Fix secret customer flow", AgentPhase: "finalize", AgentIteration: 3,
		PassedChecks: []string{"test"}, QualityStatus: "ready", QualityScore: 100,
	}
	symbols := agentCheckpointSymbols(state)
	joined := strings.Join(symbols, " ")
	if !strings.Contains(joined, agentCheckpointMarker) || !strings.Contains(joined, "task:") {
		t.Fatalf("checkpoint markers missing: %#v", symbols)
	}
	for _, forbidden := range []string{"secret customer flow", "ready", "passed", "test"} {
		if strings.Contains(strings.ToLower(joined), forbidden) {
			t.Fatalf("checkpoint leaked unsafe/stale completion data %q: %#v", forbidden, symbols)
		}
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

func TestLongTermMemoryInputPersistsOnlyCompactTaskFacts(t *testing.T) {
	input := longTermMemoryInput("user", "session", "workspace", "verify", operationInvocation{OperationID: "verify.changes"}, taskstate.State{
		Task:          "Fix login",
		Branch:        "feat/login",
		TouchedFiles:  []string{"a.go"},
		RecentChecks:  []string{"verify.changes"},
		QualityStatus: "ready",
	}, &mcp.CallToolResult{})
	if input == nil || input.Level != longmemory.LevelScenario || input.WorkspaceID != "workspace" {
		t.Fatalf("unexpected long-term memory input: %#v", input)
	}
	if len(input.Files) != 1 || input.Files[0] != "a.go" || !strings.Contains(input.Summary, "Fix login") {
		t.Fatalf("expected compact task facts only, got %#v", input)
	}
	if len(input.Symbols) == 0 || input.Symbols[0] != agentCheckpointMarker {
		t.Fatalf("expected compact agent checkpoint markers, got %#v", input.Symbols)
	}
}

func TestLongTermMemoryDoesNotCallIncompleteVerificationReady(t *testing.T) {
	input := longTermMemoryInput("user", "session", "workspace", "verify", operationInvocation{OperationID: "verify.changes"}, taskstate.State{
		Task: "Fix login", AgentPhase: "verify", QualityStatus: "verifying",
	}, &mcp.CallToolResult{})
	if input == nil || input.Level != longmemory.LevelEvent {
		t.Fatalf("incomplete verification should remain an event: %#v", input)
	}
	if strings.Contains(strings.ToLower(input.Summary), "ready") || strings.Contains(strings.ToLower(input.Summary), "success") {
		t.Fatalf("incomplete verification summary overclaims readiness: %q", input.Summary)
	}
}

func TestAttachLongTermMemoryUsesCompactStructuredContent(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"results": []any{}}}
	attachLongTermMemory(result, []longmemory.Record{{ID: "mem-1", Level: longmemory.LevelScenario, Summary: "OAuth fix verified", Files: []string{"oauth.go"}, Score: .9}})
	root := result.StructuredContent.(map[string]any)
	items, ok := root["longTermMemory"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["summary"] != "OAuth fix verified" {
		t.Fatalf("unexpected long-term memory projection: %#v", root["longTermMemory"])
	}
}
