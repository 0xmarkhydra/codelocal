package taskstate

import (
	"testing"
	"time"
)

func TestStoreKeepsOperationalContext(t *testing.T) {
	store := New(8)
	state := store.Update("u1", "s1", "w1", Patch{
		Task:         "Fix OAuth login regression",
		Branch:       "feat/auth-fix",
		TouchedFiles: []string{"src/auth.ts", "src/auth.ts", "src/login.ts"},
		RecentChecks: []string{"go test ./internal/oauth"},
		RecentErrors: []string{"redirect mismatch"},
		LastAction:   "edit.replace",
	})
	if state.Task != "Fix OAuth login regression" || state.Branch != "feat/auth-fix" {
		t.Fatalf("unexpected state: %#v", state)
	}
	if len(state.TouchedFiles) != 2 {
		t.Fatalf("expected de-duplicated touched files, got %#v", state.TouchedFiles)
	}

	state = store.Update("u1", "s1", "w1", Patch{
		TouchedFiles: []string{"src/oauth.ts"},
		RecentChecks: []string{"go test ./..."},
		LastAction:   "verify.changes",
	})
	if state.Task == "" || state.Branch == "" {
		t.Fatal("partial update erased durable task context")
	}
	if state.TouchedFiles[0] != "src/oauth.ts" {
		t.Fatalf("new touched file should be most recent: %#v", state.TouchedFiles)
	}
}

func TestStoreSeparatesSessionsAndWorkspaces(t *testing.T) {
	store := New(8)
	store.Update("u", "thread-a", "repo", Patch{Task: "Task A"})
	store.Update("u", "thread-b", "repo", Patch{Task: "Task B"})
	store.Update("u", "thread-a", "other", Patch{Task: "Task C"})
	if state, _ := store.Get("u", "thread-a", "repo"); state.Task != "Task A" {
		t.Fatalf("session state leaked: %#v", state)
	}
	if state, _ := store.Get("u", "thread-b", "repo"); state.Task != "Task B" {
		t.Fatalf("session state leaked: %#v", state)
	}
	if state, _ := store.Get("u", "thread-a", "other"); state.Task != "Task C" {
		t.Fatalf("workspace state leaked: %#v", state)
	}
}

func TestReplaceErrorsClearsResolvedFailure(t *testing.T) {
	store := New(8)
	store.Update("u", "s", "w", Patch{RecentErrors: []string{"old failure"}})
	state := store.Update("u", "s", "w", Patch{ReplaceErrors: true})
	if len(state.RecentErrors) != 0 {
		t.Fatalf("expected resolved errors to clear: %#v", state.RecentErrors)
	}
}

func TestStoreExpiresStaleTaskMemory(t *testing.T) {
	store := NewWithTTL(8, time.Millisecond)
	store.Update("u", "s", "w", Patch{Task: "stale task"})
	time.Sleep(3 * time.Millisecond)
	if state, ok := store.Get("u", "s", "w"); ok || state.Task != "" {
		t.Fatalf("expected stale state to expire, got %#v", state)
	}
}
