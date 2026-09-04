package orchestration

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestCodingKernelDurableLifecycleRecoveryAndCompletion(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	runtime, err := NewShadowRuntime(store, "workspace", "task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	lead, err := runtime.Graph().Register(agentruntime.AgentIdentity{ID: "lead", TaskID: "task", Role: agentruntime.RoleLead, EngineID: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	lead, err = runtime.Graph().TransitionCAS(lead.ID, lead.Revision, agentruntime.AgentActive, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := NewCodingKernel(store, "workspace", "task", runtime)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := kernel.Snapshot()
	if snapshot.Stage != KernelUnderstand || snapshot.Revision != 1 {
		t.Fatalf("initial kernel = %#v", snapshot)
	}
	if _, err := kernel.Advance(snapshot.Revision, KernelPlan, "skip locate"); !errors.Is(err, ErrInvalidKernelTransition) {
		t.Fatalf("expected skipped-stage rejection, got %v", err)
	}
	snapshot, err = kernel.Advance(snapshot.Revision, KernelLocate, "repo mapped")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = kernel.Advance(snapshot.Revision, KernelPlan, "symbols located")
	if err != nil {
		t.Fatal(err)
	}
	failedRevision := snapshot.Revision
	snapshot, decision, err := kernel.HandleFailure(snapshot.Revision, FailureObservation{OccurrenceID: "provider-1", AgentID: lead.ID, Source: "model", Code: "rate_limit", Message: "provider rate limit: secret-value-must-not-persist"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Stage != KernelRepair || snapshot.LastFailureStage != KernelPlan || decision.Action != RecoverySwitchEngine || !decision.Retryable {
		t.Fatalf("provider recovery = snapshot=%#v decision=%#v", snapshot, decision)
	}
	if _, err := kernel.Advance(failedRevision, KernelAct, "stale caller"); !errors.Is(err, ErrStaleKernelRevision) {
		t.Fatalf("expected stale revision rejection, got %v", err)
	}

	recoveredRuntime, err := NewShadowRuntime(store, "workspace", "task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	recoveredKernel, err := NewCodingKernel(store, "workspace", "task", recoveredRuntime)
	if err != nil {
		t.Fatal(err)
	}
	recovered := recoveredKernel.Snapshot()
	if recovered.Stage != KernelRepair || recovered.Revision != snapshot.Revision || recovered.LastFailureStage != KernelPlan {
		t.Fatalf("recovered kernel = %#v", recovered)
	}
	recovered, err = recoveredKernel.ResumeRecovery(recovered.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Stage != KernelPlan || recovered.Cycle != 1 {
		t.Fatalf("resume recovery = %#v", recovered)
	}
	for _, next := range []KernelStage{KernelAct, KernelObserve, KernelVerify, KernelReflect} {
		recovered, err = recoveredKernel.Advance(recovered.Revision, next, "stage passed")
		if err != nil {
			t.Fatalf("advance to %s: %v", next, err)
		}
	}
	lead, ok := recoveredRuntime.Graph().Agent(lead.ID)
	if !ok {
		t.Fatal("recovered lead missing")
	}
	completed, err := recoveredKernel.Complete(recovered.Revision, lead.ID, lead.Revision, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Stage != KernelComplete || completed.LastReasonCode != "verified_complete" {
		t.Fatalf("completed kernel = %#v", completed)
	}

	finalRuntime, err := NewShadowRuntime(store, "workspace", "task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	finalKernel, err := NewCodingKernel(store, "workspace", "task", finalRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if finalKernel.Snapshot().Stage != KernelComplete || finalKernel.Snapshot().Revision != completed.Revision {
		t.Fatalf("final replay = %#v", finalKernel.Snapshot())
	}
	finalLead, ok := finalRuntime.Graph().Agent(lead.ID)
	if !ok || finalLead.Status != agentruntime.AgentCompleted {
		t.Fatalf("final lead = %#v ok=%v", finalLead, ok)
	}

	events, err := store.List("workspace", "task", 0, 5000)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type != eventFailureDecision && event.Type != eventKernelFailure {
			continue
		}
		if containsSecretValue(event.Payload, "secret-value-must-not-persist") {
			t.Fatalf("raw failure text leaked into durable event: %#v", event)
		}
	}
}

func TestCodingKernelBlockedResumeReturnsToInterruptedStage(t *testing.T) {
	runtime, err := NewShadowRuntime(nil, "workspace", "blocked-task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := NewCodingKernel(nil, "workspace", "blocked-task", runtime)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := kernel.Snapshot()
	snapshot, err = kernel.Advance(snapshot.Revision, KernelLocate, "start locate")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, decision, err := kernel.HandleFailure(snapshot.Revision, FailureObservation{OccurrenceID: "policy-1", Code: "policy_denied", Message: "approval required"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Stage != KernelBlocked || snapshot.LastFailureStage != KernelLocate || !decision.RequiresUser {
		t.Fatalf("blocked snapshot = %#v decision=%#v", snapshot, decision)
	}
	snapshot, err = kernel.ResumeBlocked(snapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Stage != KernelLocate || snapshot.Cycle != 1 {
		t.Fatalf("blocked resume = %#v", snapshot)
	}
}

func TestCodingKernelRejectsCorruptDurableSnapshot(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	runtime, err := NewShadowRuntime(store, "workspace", "corrupt-task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Append("workspace", "corrupt-task", runtimeevents.Event{
		Type:           eventKernelInitialized,
		TaskID:         "corrupt-task",
		IdempotencyKey: "kernel:init",
		Payload:        map[string]any{"kernel": map[string]any{"revision": 0, "stage": "complete"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCodingKernel(store, "workspace", "corrupt-task", runtime); !errors.Is(err, ErrInvalidCodingKernel) {
		t.Fatalf("expected corrupt kernel rejection, got %v", err)
	}
}

func containsSecretValue(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return typed == needle || (needle != "" && len(typed) >= len(needle) && stringContains(typed, needle))
	case map[string]any:
		for _, child := range typed {
			if containsSecretValue(child, needle) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsSecretValue(child, needle) {
				return true
			}
		}
	}
	return false
}

func stringContains(value, needle string) bool {
	if needle == "" {
		return false
	}
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
