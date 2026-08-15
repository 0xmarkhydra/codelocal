package mcpgateway

import (
	"context"
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

func TestAgentPatchPersistsCommandSpecificOpaqueVerificationID(t *testing.T) {
	result := &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	command := "go test ./internal/foo --token super-secret-value"
	patch := agentPatchForOperation(operationInvocation{OperationID: "terminal.run"}, map[string]any{
		"command": command,
	}, result, taskstate.State{AgentPhase: "verify"})
	want := orchestration.CheckID(command)
	if len(patch.PassedChecks) != 1 || patch.PassedChecks[0] != want {
		t.Fatalf("expected command-specific opaque check ID, got %#v", patch.PassedChecks)
	}
	if strings.Contains(strings.Join(patch.PassedChecks, " "), "secret") || strings.Contains(strings.Join(patch.PassedChecks, " "), "./internal/foo") {
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

func TestCompactLongTermMemoryEnforcesPromptBudgetAndDeduplicates(t *testing.T) {
	longSummary := strings.Repeat("architectural decision with useful evidence ", 80)
	records := []longmemory.Record{
		{ID: "one", Summary: longSummary, Files: []string{"a.go", "b.go", "c.go", "d.go", "e.go"}},
		{ID: "duplicate", Summary: longSummary},
		{ID: "two", Summary: strings.Repeat("repository-specific context ", 70)},
		{ID: "three", Summary: strings.Repeat("another useful memory ", 70)},
		{ID: "four", Summary: strings.Repeat("fourth useful memory ", 70)},
		{ID: "five", Summary: "must be excluded by item budget"},
	}
	compact := compactLongTermMemory(records)
	if len(compact) == 0 || len(compact) > longTermMemoryMaxItems {
		t.Fatalf("unexpected compact memory count: %d", len(compact))
	}
	seen := map[string]struct{}{}
	for _, record := range compact {
		if runeLen(record.Summary) > longTermMemorySummaryMaxChar {
			t.Fatalf("summary exceeded per-item budget: %d", runeLen(record.Summary))
		}
		if len(record.Files) > 4 {
			t.Fatalf("file metadata exceeded compact limit: %#v", record.Files)
		}
		key := strings.ToLower(strings.Join(strings.Fields(record.Summary), " "))
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate memory survived compaction: %q", key)
		}
		seen[key] = struct{}{}
	}
}

type recordingLongTermMemory struct {
	inputs       []longmemory.IngestInput
	recallInputs []longmemory.RecallInput
	recalls      []longmemory.Record
}

func (m *recordingLongTermMemory) Ingest(_ context.Context, input longmemory.IngestInput) (longmemory.Record, error) {
	m.inputs = append(m.inputs, input)
	return longmemory.Record{
		ID:           longmemory.IdempotencyKey(input.UserID, string(input.Scope), input.WorkspaceID, input.ProjectID, input.RepositoryID, input.Kind, input.Summary)[:32],
		UserID:       input.UserID,
		WorkspaceID:  input.WorkspaceID,
		ProjectID:    input.ProjectID,
		RepositoryID: input.RepositoryID,
		Scope:        input.Scope,
		TaskID:       input.TaskID,
		Level:        input.Level,
		Kind:         input.Kind,
		SourceType:   input.SourceType,
		Summary:      input.Summary,
	}, nil
}

func (m *recordingLongTermMemory) Recall(_ context.Context, input longmemory.RecallInput) ([]longmemory.Record, error) {
	m.recallInputs = append(m.recallInputs, input)
	return append([]longmemory.Record(nil), m.recalls...), nil
}

func TestRememberConversationMemoryStoresGlobalSanitizedFact(t *testing.T) {
	memory := &recordingLongTermMemory{}
	service := &Service{Memory: memory}
	result, err := service.rememberConversationMemory(context.Background(), "user-1", "thread-1", map[string]any{
		"memories": []any{map[string]any{
			"kind": "preference", "scope": "global", "summary": "Prefers Go; api_key=super-secret-value", "importance": .85, "confidence": .95,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.IsError {
		t.Fatalf("remember returned error result: %#v", result)
	}
	if len(memory.inputs) != 1 {
		t.Fatalf("ingest count=%d want=1", len(memory.inputs))
	}
	input := memory.inputs[0]
	if input.Scope != longmemory.ScopeGlobal || input.WorkspaceID != "" || input.Kind != "preference" || input.SourceType != "conversation" {
		t.Fatalf("unexpected durable memory input: %#v", input)
	}
	if strings.Contains(input.Summary, "super-secret-value") || !strings.Contains(input.Summary, "[REDACTED]") {
		t.Fatalf("conversation memory did not redact secret-like content: %q", input.Summary)
	}
	if input.Level != longmemory.LevelWorkspace || input.Importance != .85 || input.Confidence != .95 {
		t.Fatalf("unexpected memory ranking metadata: %#v", input)
	}
}

func TestRememberConversationMemoryStableKeyReplacesMutableFactIdentity(t *testing.T) {
	memory := &recordingLongTermMemory{}
	service := &Service{Memory: memory}
	for _, summary := range []string{"Target 10,000 users by year end", "Target 20,000 users by year end"} {
		result, err := service.rememberConversationMemory(context.Background(), "user-1", "thread-1", map[string]any{
			"memories": []any{map[string]any{
				"kind": "goal", "key": "user.goal.codelocal_users", "scope": "global", "summary": summary,
			}},
		})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("remember stable fact failed: result=%#v err=%v", result, err)
		}
	}
	if len(memory.inputs) != 2 {
		t.Fatalf("ingest count=%d want=2", len(memory.inputs))
	}
	if memory.inputs[0].IdempotencyKey == "" || memory.inputs[0].IdempotencyKey != memory.inputs[1].IdempotencyKey {
		t.Fatalf("stable key should preserve one durable identity across updates: %#v", memory.inputs)
	}
	if memory.inputs[0].Summary == memory.inputs[1].Summary {
		t.Fatal("test must exercise a changed mutable value")
	}
}

func TestRecallConversationMemorySupportsGlobalMemoryWithoutWorkspace(t *testing.T) {
	memory := &recordingLongTermMemory{recalls: []longmemory.Record{{
		ID: "global-1", UserID: "user-1", Scope: longmemory.ScopeGlobal, Level: longmemory.LevelWorkspace,
		Kind: "preference", SourceType: "conversation", Summary: "Prefers concise answers", Score: .9,
	}}}
	service := &Service{Memory: memory}
	result, err := service.recallConversationMemory(context.Background(), "user-1", "thread-1", map[string]any{"query": "answer preference"})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("global recall failed without workspace: result=%#v err=%v", result, err)
	}
	if len(memory.recallInputs) != 1 || memory.recallInputs[0].WorkspaceID != "" {
		t.Fatalf("global-only recall should not require a workspace: %#v", memory.recallInputs)
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("recall missing structured content: %#v", result.StructuredContent)
	}
	items, ok := root["memories"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected recalled memories: %#v", root["memories"])
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["summary"] != "Prefers concise answers" {
		t.Fatalf("unexpected recalled memory item: %#v", items[0])
	}
}

func TestRememberConversationMemoryRequiresWorkspaceForWorkspaceScope(t *testing.T) {
	memory := &recordingLongTermMemory{}
	service := &Service{Memory: memory}
	result, err := service.rememberConversationMemory(context.Background(), "user-1", "thread-1", map[string]any{
		"memories": []any{map[string]any{"kind": "decision", "scope": "workspace", "summary": "Use Go runtime"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("workspace memory without selection should be rejected: %#v", result)
	}
	if len(memory.inputs) != 0 {
		t.Fatalf("rejected memory should not partially ingest: %#v", memory.inputs)
	}
}

func BenchmarkProjectBrainContextCompactionBaseline(b *testing.B) {
	records := make([]longmemory.Record, 64)
	for i := range records {
		records[i] = longmemory.Record{
			ID:         "memory",
			Scope:      longmemory.ScopeProject,
			Level:      longmemory.LevelScenario,
			Kind:       "project_fact",
			SourceType: "task",
			Summary:    strings.Repeat("project context fact with enough detail to exercise compaction ", 8) + string(rune(0x1000+i)),
			Branch:     "dev",
			Files:      []string{"internal/payment/service.go", "internal/payment/service_test.go", "internal/payment/repository.go"},
			Score:      1 - float64(i)/100,
		}
	}
	selected := compactLongTermMemory(records)
	selectedRunes := 0
	for _, record := range selected {
		selectedRunes += runeLen(record.Summary)
	}
	b.ReportMetric(float64(len(records)), "candidate-memories")
	b.ReportMetric(float64(len(selected)), "selected-memories")
	b.ReportMetric(float64(selectedRunes), "selected-summary-runes")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = compactLongTermMemory(records)
	}
}
