package taskexecution

import "testing"

func TestTaskResumeRoundTrip(t *testing.T) {
	bundle := Bundle{SchemaVersion: 1, ID: "exec_x", TaskID: "task-a", WorkspaceKey: "workspace", State: StateReady}
	snapshot, err := ExportTaskResume(bundle, 42)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Digest == "" || snapshot.EventHighWater != 42 {
		t.Fatalf("snapshot mismatch: %+v", snapshot)
	}
	imported, highWater, err := ImportTaskResume(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if imported.TaskID != "task-a" || highWater != 42 {
		t.Fatalf("import mismatch: %+v %d", imported, highWater)
	}
}

func TestTaskResumeRejectsTampering(t *testing.T) {
	bundle := Bundle{SchemaVersion: 1, ID: "exec_x", TaskID: "task-a", WorkspaceKey: "workspace", State: StateReady}
	snapshot, err := ExportTaskResume(bundle, 42)
	if err != nil {
		t.Fatal(err)
	}
	tampered := snapshot
	tampered.EventHighWater = 7
	if _, _, err := ImportTaskResume(tampered); err == nil {
		t.Fatal("rewound high-water mark accepted")
	}
	tampered = snapshot
	tampered.Bundle.State = StatePreparing
	if _, _, err := ImportTaskResume(tampered); err == nil {
		t.Fatal("swapped bundle accepted")
	}
	tampered = snapshot
	mismatched := snapshot
	mismatched.TaskID = "task-b"
	if _, _, err := ImportTaskResume(mismatched); err == nil {
		t.Fatal("task id mismatch accepted")
	}
	if _, _, err := ImportTaskResume(TaskResumeSnapshot{}); err == nil {
		t.Fatal("empty snapshot accepted")
	}
}
