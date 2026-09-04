package orchestration

import "testing"

func TestDecideProjectOSRecoveryBoundsRetries(t *testing.T) {
	if got := DecideProjectOSRecovery(ProjectOSFailure{Kind: "transient"}); got != ProjectOSRetryOnce {
		t.Fatalf("first transient should retry once, got %s", got)
	}
	if got := DecideProjectOSRecovery(ProjectOSFailure{Kind: "transient", RetryCount: 1}); got != ProjectOSRepairSubtask {
		t.Fatalf("second failure should become repair subtask, got %s", got)
	}
	if got := DecideProjectOSRecovery(ProjectOSFailure{Kind: "transient", Duplicate: true}); got != ProjectOSDoNotRetry {
		t.Fatalf("duplicate delivery must not retry, got %s", got)
	}
	if got := DecideProjectOSRecovery(ProjectOSFailure{Kind: "execution_failed", PermissionErr: true}); got != ProjectOSWaitingHuman {
		t.Fatalf("permission failure must escalate, got %s", got)
	}
}

func TestShouldEscalateToHumanHidesRoutineFailures(t *testing.T) {
	if ShouldEscalateToHuman("RUNNING", 0, "") {
		t.Fatal("running task should not escalate")
	}
	if ShouldEscalateToHuman("RETRYING", 1, "") {
		t.Fatal("first retry should not escalate")
	}
	if !ShouldEscalateToHuman("FAILED", 1, "") {
		t.Fatal("failed task should escalate")
	}
	if !ShouldEscalateToHuman("WAITING_HUMAN", 0, "") {
		t.Fatal("waiting-human task should escalate")
	}
}
