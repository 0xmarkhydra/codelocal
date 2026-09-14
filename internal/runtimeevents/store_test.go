package runtimeevents

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStoreAppendOrdersAndDeduplicates(t *testing.T) {
	store := NewStore(t.TempDir())

	first, appended, err := store.Append("workspace-a", "task-a", Event{
		Type:           "task.started",
		IdempotencyKey: "start",
		Payload:        map[string]any{"objective": "fix auth"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !appended || first.Sequence != 1 || first.ID == "" {
		t.Fatalf("unexpected first event: appended=%v event=%+v", appended, first)
	}

	second, appended, err := store.Append("workspace-a", "task-a", Event{
		Type:           "agent.spawned",
		IdempotencyKey: "agent-1",
		AgentID:        "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !appended || second.Sequence != 2 {
		t.Fatalf("unexpected second event: appended=%v event=%+v", appended, second)
	}

	duplicate, appended, err := store.Append("workspace-a", "task-a", Event{
		Type:           "task.started",
		IdempotencyKey: "start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended {
		t.Fatal("duplicate idempotency key appended another event")
	}
	if duplicate.ID != first.ID || duplicate.Sequence != first.Sequence {
		t.Fatalf("duplicate did not return original event: %+v vs %+v", duplicate, first)
	}

	events, err := store.List("workspace-a", "task-a", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("unexpected ordered events: %+v", events)
	}
}

func TestStoreReplayFromSnapshot(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, kind := range []string{"task.started", "context.compiled", "agent.started"} {
		if _, _, err := store.Append("workspace-a", "task-a", Event{Type: kind}); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := store.SaveSnapshot("workspace-a", "task-a", Snapshot{
		Sequence: 2,
		State: map[string]any{
			"status": "running",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Sequence != 2 || snapshot.CreatedAt.IsZero() {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	loaded, found, events, err := store.Replay("workspace-a", "task-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Sequence != 2 {
		t.Fatalf("snapshot not replayed: found=%v snapshot=%+v", found, loaded)
	}
	if len(events) != 1 || events[0].Sequence != 3 || events[0].Type != "agent.started" {
		t.Fatalf("unexpected replay tail: %+v", events)
	}
}

func TestStoreRejectsSnapshotAheadOfLog(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, _, err := store.Append("workspace-a", "task-a", Event{Type: "task.started"}); err != nil {
		t.Fatal(err)
	}
	_, err := store.SaveSnapshot("workspace-a", "task-a", Snapshot{Sequence: 2})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent, got %v", err)
	}
}

func TestStoreDetectsCorruptLog(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if _, _, err := store.Append("workspace-a", "task-a", Event{Type: "task.started"}); err != nil {
		t.Fatal(err)
	}

	path := store.eventPath("workspace-a", "task-a")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{broken-json}\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = store.List("workspace-a", "task-a", 0, 10)
	if !errors.Is(err, ErrCorruptLog) {
		t.Fatalf("expected ErrCorruptLog, got %v", err)
	}
}

func TestStoreUsesPrivateHashedPaths(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if _, _, err := store.Append("secret-workspace-key", "task-a", Event{Type: "task.started"}); err != nil {
		t.Fatal(err)
	}
	path := store.eventPath("secret-workspace-key", "task-a")
	if filepath.Base(filepath.Dir(filepath.Dir(path))) == "secret-workspace-key" {
		t.Fatalf("workspace key leaked into state path: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("event log is not private: %o", info.Mode().Perm())
	}
}
