package taskstatus

import "testing"

import "github.com/0xmarkhydra/codelocal/internal/orchestration"
import "github.com/0xmarkhydra/codelocal/internal/taskexecution"
import "github.com/0xmarkhydra/codelocal/internal/usage"

func TestSummarizeTaskHealthy(t *testing.T) {
	bundle := taskexecution.Bundle{TaskID: "task-a", WorkspaceKey: "workspace", State: taskexecution.StateReady}
	budget := usage.BudgetDecision{State: usage.BudgetWithin, RemainingTotal: 100}
	graph := orchestration.SummarizeVerification(
		[]orchestration.VerificationRequirement{{ID: "unit-1", Category: "unit", Required: true}},
		[]orchestration.VerificationEvidence{{CheckID: "unit-1", Status: orchestration.VerificationPassed}},
	)
	snapshot := SummarizeTask(bundle, budget, graph)
	if !snapshot.Healthy || !snapshot.VerificationReady || len(snapshot.Lanes) != 1 {
		t.Fatalf("healthy snapshot mismatch: %+v", snapshot)
	}
	hard := usage.BudgetDecision{State: usage.BudgetHard, Reason: "overspent"}
	if snapshot := SummarizeTask(bundle, hard, graph); snapshot.Healthy {
		t.Fatal("hard-limited budget reported healthy")
	}
	pending := orchestration.SummarizeVerification(
		[]orchestration.VerificationRequirement{{ID: "unit-1", Required: true}}, nil)
	if snapshot := SummarizeTask(bundle, budget, pending); snapshot.Healthy {
		t.Fatal("unverified task reported healthy")
	}
}
