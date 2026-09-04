package orchestration

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestVerificationGateRequiresDurablePassingEvidence(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	plan := VerificationPlan{Mode: "focused", Checks: []VerificationCheck{
		{Key: "diff", Command: "git diff --check", Required: true, Scope: "changed files"},
		{Key: "test", Command: "go test ./internal/orchestration", Required: true, Scope: "orchestration"},
	}}
	gate, err := NewVerificationGate(store, "workspace", "task", plan)
	if err != nil {
		t.Fatal(err)
	}
	if gate.CanComplete() {
		t.Fatal("task must not complete before required evidence exists")
	}
	snapshot := gate.Snapshot()
	if len(snapshot.Requirements) != 2 {
		t.Fatalf("requirements = %#v", snapshot.Requirements)
	}
	first := snapshot.Requirements[0].ID
	second := snapshot.Requirements[1].ID
	snapshot, err = gate.Record(snapshot.Revision, VerificationEvidence{CheckID: first, Status: VerificationPassed, ArtifactRef: "artifact://diff"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Record(snapshot.Revision-1, VerificationEvidence{CheckID: second, Status: VerificationPassed}); !errors.Is(err, ErrStaleVerification) {
		t.Fatalf("expected stale evidence rejection, got %v", err)
	}
	snapshot, err = gate.Record(snapshot.Revision, VerificationEvidence{CheckID: second, Status: VerificationPassed, ArtifactRef: "artifact://tests"})
	if err != nil {
		t.Fatal(err)
	}
	if !gate.CanComplete() {
		t.Fatal("all required checks passed but completion is blocked")
	}

	recovered, err := NewVerificationGate(store, "workspace", "task", VerificationPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.CanComplete() || recovered.Snapshot().Revision != snapshot.Revision {
		t.Fatalf("recovered verification = %#v", recovered.Snapshot())
	}
}

func TestVerificationGateFailureMustBeRepaired(t *testing.T) {
	plan := VerificationPlan{Checks: []VerificationCheck{{Command: "go test ./...", Required: true}}}
	gate, err := NewVerificationGate(nil, "workspace", "task", plan)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := gate.Snapshot()
	id := snapshot.Requirements[0].ID
	exit := 1
	snapshot, err = gate.Record(snapshot.Revision, VerificationEvidence{CheckID: id, Status: VerificationFailed, ExitCode: &exit, FailureSignature: "tests_failed"})
	if err != nil {
		t.Fatal(err)
	}
	if gate.CanComplete() || len(gate.FailedRequired()) != 1 {
		t.Fatalf("failed gate state = %#v", gate.Snapshot())
	}
	snapshot, err = gate.Record(snapshot.Revision, VerificationEvidence{CheckID: id, Status: VerificationRunning})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Record(snapshot.Revision, VerificationEvidence{CheckID: id, Status: VerificationPassed}); err != nil {
		t.Fatal(err)
	}
	if !gate.CanComplete() {
		t.Fatal("repaired verification should permit completion")
	}
}
