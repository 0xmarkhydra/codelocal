package orchestration

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestShadowRuntimeDurableDispatchVerifyAndFinalize(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	plan := VerificationPlan{Checks: []VerificationCheck{{Command: "go test ./internal/orchestration", Required: true}}}
	runtime, err := NewShadowRuntime(store, "workspace", "task", plan)
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
	worker, _, err := runtime.Graph().Spawn(lead.ID, agentruntime.AgentIdentity{ID: "worker", TaskID: "task", Role: agentruntime.RoleImplementer, EngineID: "auto"}, agentruntime.EdgeSpawn)
	if err != nil {
		t.Fatal(err)
	}
	node, err := runtime.DAG().Add(TaskNode{ID: "fix", Subject: "Fix bug", OwnerAgentID: worker.ID})
	if err != nil {
		t.Fatal(err)
	}
	assignments, err := runtime.PrepareRunnableAssignments(lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].Content != "task-node:fix" {
		t.Fatalf("assignments = %#v", assignments)
	}
	again, err := runtime.PrepareRunnableAssignments(lead.ID)
	if err != nil || len(again) != 0 {
		t.Fatalf("duplicate assignment = %#v err=%v", again, err)
	}
	if _, err := runtime.Mailbox().Deliver(worker.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	worker, err = runtime.Graph().TransitionCAS(worker.ID, worker.Revision, agentruntime.AgentActive, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	node, err = runtime.StartNode(node.ID, node.Revision, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ReadyToFinalize(lead.ID) {
		t.Fatal("runtime finalized while worker/node/verification were incomplete")
	}
	worker, err = runtime.Graph().TransitionCAS(worker.ID, worker.Revision, agentruntime.AgentCompleted, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	node, err = runtime.CompleteNode(node.ID, node.Revision, worker.ID)
	if err != nil || node.Status != TaskNodeCompleted {
		t.Fatalf("complete node = %#v err=%v", node, err)
	}
	snapshot := runtime.Verification().Snapshot()
	if len(snapshot.Requirements) != 1 {
		t.Fatalf("verification = %#v", snapshot)
	}
	if _, err := runtime.Verification().Record(snapshot.Revision, VerificationEvidence{CheckID: snapshot.Requirements[0].ID, Status: VerificationPassed, ArtifactRef: "artifact://tests"}); err != nil {
		t.Fatal(err)
	}
	if !runtime.ReadyToFinalize(lead.ID) {
		t.Fatal("runtime should be ready after successful agents, DAG and verification")
	}
	lead, err = runtime.FinalizeLead(lead.ID, lead.Revision, time.Time{})
	if err != nil || lead.Status != agentruntime.AgentCompleted {
		t.Fatalf("finalize lead = %#v err=%v", lead, err)
	}

	recovered, err := NewShadowRuntime(store, "workspace", "task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.ReadyToFinalize(lead.ID) {
		t.Fatal("recovered runtime lost finalization evidence")
	}
	if assignments, err := recovered.PrepareRunnableAssignments(lead.ID); err != nil || len(assignments) != 0 {
		t.Fatalf("recovery re-queued completed work: %#v err=%v", assignments, err)
	}
}

func TestShadowRuntimeRejectsStaleOrPrematureCompletion(t *testing.T) {
	runtime, err := NewShadowRuntime(nil, "workspace", "task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	lead, _ := runtime.Graph().Register(agentruntime.AgentIdentity{ID: "lead", TaskID: "task", Role: agentruntime.RoleLead, EngineID: "auto"})
	lead, _ = runtime.Graph().TransitionCAS(lead.ID, lead.Revision, agentruntime.AgentActive, time.Time{})
	worker, _, _ := runtime.Graph().Spawn(lead.ID, agentruntime.AgentIdentity{ID: "worker", TaskID: "task", EngineID: "auto"}, agentruntime.EdgeSpawn)
	node, _ := runtime.DAG().Add(TaskNode{ID: "work", Subject: "Work", OwnerAgentID: worker.ID})
	if _, err := runtime.StartNode(node.ID, node.Revision, worker.ID); !errors.Is(err, ErrShadowRuntimeAgent) {
		t.Fatalf("inactive worker started node: %v", err)
	}
	worker, _ = runtime.Graph().TransitionCAS(worker.ID, worker.Revision, agentruntime.AgentActive, time.Time{})
	node, err = runtime.StartNode(node.ID, node.Revision, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CompleteNode(node.ID, node.Revision, worker.ID); !errors.Is(err, ErrShadowRuntimeAgent) {
		t.Fatalf("active worker completed node before agent completion: %v", err)
	}
	if _, err := runtime.Graph().TransitionCAS(worker.ID, worker.Revision-1, agentruntime.AgentCompleted, time.Time{}); !errors.Is(err, agentruntime.ErrStaleAgentRevision) {
		t.Fatalf("expected stale agent revision, got %v", err)
	}
	if _, err := runtime.FinalizeLead(lead.ID, lead.Revision, time.Time{}); !errors.Is(err, ErrShadowRuntimeNotReady) {
		t.Fatalf("premature finalization = %v", err)
	}
}
