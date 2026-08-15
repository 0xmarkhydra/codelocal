package orchestration

import "testing"

func TestAdvanceLoopMovesEditIntoVerification(t *testing.T) {
	state := AdvanceLoop(LoopState{Phase: "inspect", Iteration: 2}, LoopEvent{Operation: "edit.apply", Success: true})
	if state.Phase != "verify" || state.Iteration != 3 || state.NextAction == "" {
		t.Fatalf("unexpected edit transition: %#v", state)
	}
}

func TestAdvanceLoopBoundsRecoveryAndDoesNotRetryPermission(t *testing.T) {
	stale := AdvanceLoop(LoopState{RecoveryAttempts: 0}, LoopEvent{Operation: "computer.click", Success: false, Failure: "stale element after navigation"})
	if stale.Phase != "recover" || stale.RecoveryAttempts != 1 {
		t.Fatalf("expected bounded stale-UI recovery: %#v", stale)
	}
	second := AdvanceLoop(stale, LoopEvent{Operation: "computer.click", Success: false, Failure: "stale element after navigation"})
	third := AdvanceLoop(second, LoopEvent{Operation: "computer.click", Success: false, Failure: "stale element after navigation"})
	if second.RecoveryAttempts != 2 || third.RecoveryAttempts != 2 {
		t.Fatalf("recovery attempts exceeded classifier bound: second=%#v third=%#v", second, third)
	}

	permission := AdvanceLoop(LoopState{}, LoopEvent{Operation: "computer.click", Success: false, Failure: "approval required"})
	if permission.RecoveryAttempts != 0 || permission.Phase != "recover" {
		t.Fatalf("permission failure must not auto-retry: %#v", permission)
	}
}

func TestAdvanceLoopTreatsVerificationCommandAsEvidence(t *testing.T) {
	state := AdvanceLoop(LoopState{Phase: "verify", RecoveryAttempts: 1}, LoopEvent{Operation: "terminal.run", Success: true, CheckKey: "test"})
	if state.RecoveryAttempts != 0 || state.Phase != "verify" {
		t.Fatalf("successful check should reset recovery state: %#v", state)
	}
}
