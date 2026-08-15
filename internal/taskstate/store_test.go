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

func TestStoreReturnsDefensiveSliceCopies(t *testing.T) {
	store := New(8)
	state := store.Update("u", "s", "w", Patch{TouchedFiles: []string{"a.go"}, RecentChecks: []string{"verify.changes"}})
	state.TouchedFiles[0] = "mutated.go"
	state.RecentChecks[0] = "mutated.check"
	fresh, ok := store.Get("u", "s", "w")
	if !ok {
		t.Fatal("expected stored state")
	}
	if fresh.TouchedFiles[0] != "a.go" || fresh.RecentChecks[0] != "verify.changes" {
		t.Fatalf("caller mutation leaked into store: %#v", fresh)
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

func TestLatestTaskRecoversAcrossSessionRotation(t *testing.T) {
	store := New(8)
	store.Update("u", "old-session", "w", Patch{Task: "Fix OAuth", Branch: "feat/oauth", TouchedFiles: []string{"oauth.go"}})
	store.Update("u", "new-session", "w", Patch{LastAction: "verify.changes"})
	latest, ok := store.LatestTask("u", "w", 30*time.Minute)
	if !ok || latest.Task != "Fix OAuth" || latest.SessionID != "old-session" {
		t.Fatalf("expected cross-session task recovery, got %#v ok=%v", latest, ok)
	}
	if len(latest.TouchedFiles) != 1 || latest.TouchedFiles[0] != "oauth.go" {
		t.Fatalf("expected task context copy, got %#v", latest.TouchedFiles)
	}
}

func TestLatestTaskHonorsMaxAgeAndWorkspaceIsolation(t *testing.T) {
	store := New(8)
	store.Update("u", "s", "w", Patch{Task: "stale"})
	key := stateKey("u", "s", "w")
	store.mu.Lock()
	state := store.states[key]
	state.UpdatedAt = time.Now().UTC().Add(-45 * time.Minute)
	store.states[key] = state
	store.mu.Unlock()
	if state, ok := store.LatestTask("u", "w", 30*time.Minute); ok || state.Task != "" {
		t.Fatalf("expected stale latest task to be ignored, got %#v", state)
	}
	if _, ok := store.LatestTask("u", "other", time.Hour); ok {
		t.Fatal("latest task leaked across workspace")
	}
}

func TestStorePersistsAgentLoopAndQualityState(t *testing.T) {
	store := New(8)
	iteration, recovery, regression, score := 3, 1, 0, 92
	seen, diff := true, true
	state := store.Update("u", "s", "w", Patch{
		Task:                  "Fix planner",
		AgentPhase:            "verify",
		AgentIteration:        &iteration,
		RecoveryAttempts:      &recovery,
		LastOutcome:           "succeeded",
		NextAction:            "run targeted tests",
		PassedChecks:          []string{"diff-check", "test"},
		RequiredChecks:        []string{"diff-check", "test"},
		ReplaceRequiredChecks: true,
		VerificationSeen:      &seen,
		DiagnosticRegression:  &regression,
		DiffObserved:          &diff,
		QualityScore:          &score,
		QualityStatus:         "ready",
	})
	if state.AgentPhase != "verify" || state.AgentIteration != 3 || state.RecoveryAttempts != 1 {
		t.Fatalf("agent loop state was not persisted: %#v", state)
	}
	if state.QualityStatus != "ready" || state.QualityScore != 92 || len(state.PassedChecks) != 2 || len(state.RequiredChecks) != 2 {
		t.Fatalf("quality evidence was not persisted: %#v", state)
	}
}

func TestNewTaskResetsAgentLoopEvidence(t *testing.T) {
	store := New(8)
	iteration, score := 4, 95
	seen := true
	store.Update("u", "s", "w", Patch{Task: "Task A", AgentPhase: "verify", AgentIteration: &iteration, PassedChecks: []string{"test"}, VerificationSeen: &seen, QualityScore: &score, QualityStatus: "ready"})
	state := store.Update("u", "s", "w", Patch{Task: "Task B"})
	if state.Task != "Task B" || state.AgentPhase != "plan" || state.AgentIteration != 0 || len(state.PassedChecks) != 0 || state.VerificationSeen || state.QualityScore != 0 {
		t.Fatalf("new task inherited stale agent evidence: %#v", state)
	}
}

func TestStoreExpiresStaleTaskMemory(t *testing.T) {
	store := NewWithTTL(8, time.Minute)
	store.Update("u", "s", "w", Patch{Task: "stale task"})
	key := stateKey("u", "s", "w")
	store.mu.Lock()
	state := store.states[key]
	state.UpdatedAt = time.Now().UTC().Add(-2 * time.Minute)
	store.states[key] = state
	store.mu.Unlock()
	if state, ok := store.Get("u", "s", "w"); ok || state.Task != "" {
		t.Fatalf("expected stale state to expire, got %#v", state)
	}
}
