package observability

import (
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestFlightSnapshotUsesLatestDurableTaskState(t *testing.T) {
	events := []runtimeevents.Event{
		{Sequence:1, TaskID:"task", Type:"taskdag.node_added", Timestamp:time.Unix(1,0), Payload:map[string]any{"node":map[string]any{"id":"fix","subject":"Fix login","status":"pending"}}},
		{Sequence:2, TaskID:"task", Type:"taskdag.node_updated", Timestamp:time.Unix(2,0), TraceID:"trace-1", Payload:map[string]any{"node":map[string]any{"id":"fix","subject":"Fix login","status":"running"}}},
		{Sequence:3, TaskID:"task", Type:"taskdag.node_updated", Timestamp:time.Unix(3,0), TraceID:"trace-1", Payload:map[string]any{"node":map[string]any{"id":"fix","subject":"Fix login","status":"completed"}}},
	}
	snapshot := BuildFlightSnapshot(events)
	if len(snapshot.Progress) != 1 || snapshot.Progress[0].Status != ProgressPassed || snapshot.LastSequence != 3 {
		t.Fatalf("unexpected flight snapshot: %+v", snapshot)
	}
	if len(snapshot.TraceIDs) != 1 || snapshot.TraceIDs[0] != "trace-1" { t.Fatalf("trace correlation lost: %+v", snapshot) }
}

func TestFlightSnapshotNeverMarksKernelCompleteEarly(t *testing.T) {
	events := []runtimeevents.Event{{Sequence:1, TaskID:"task", Type:"codingkernel.transitioned", Payload:map[string]any{"kernel":map[string]any{"stage":"verify"}}}}
	snapshot := BuildFlightSnapshot(events)
	if len(snapshot.Progress) != 1 || snapshot.Progress[0].Status == ProgressPassed {
		t.Fatalf("kernel verify was presented as complete: %+v", snapshot)
	}
}

func TestFlightSnapshotVerificationReflectsFailure(t *testing.T) {
	events := []runtimeevents.Event{{Sequence:1, TaskID:"task", Type:"verification.result_recorded", Payload:map[string]any{"verification":map[string]any{"results":[]any{map[string]any{"status":"passed"}, map[string]any{"status":"failed"}}}}}}
	snapshot := BuildFlightSnapshot(events)
	if len(snapshot.Progress) != 1 || snapshot.Progress[0].Status != ProgressFailed {
		t.Fatalf("verification failure hidden by projection: %+v", snapshot)
	}
}
