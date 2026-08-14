package orchestration

import "testing"

func TestClassifyFailureNeverRetriesPermission(t *testing.T) {
	advice := ClassifyFailure("approval required for desktop action")
	if advice.Kind != FailurePermission || advice.Retryable || advice.MaxRetries != 0 {
		t.Fatalf("permission failure must not auto-retry: %#v", advice)
	}
}

func TestClassifyFailureRequiresReobserveForStaleUI(t *testing.T) {
	advice := ClassifyFailure("stale element after navigation")
	if advice.Kind != FailureStaleUI || !advice.Retryable || !advice.Reobserve || advice.MaxRetries != 2 {
		t.Fatalf("unexpected stale UI advice: %#v", advice)
	}
	if !AllowRecoveryAttempt(advice, 0) || !AllowRecoveryAttempt(advice, 1) || AllowRecoveryAttempt(advice, 2) {
		t.Fatalf("retry bound is wrong: %#v", advice)
	}
}

func TestUnknownFailureDoesNotBlindRetry(t *testing.T) {
	advice := ClassifyFailure("something unusual happened")
	if advice.Kind != FailureUnknown || advice.Retryable || AllowRecoveryAttempt(advice, 0) {
		t.Fatalf("unknown failure should escalate: %#v", advice)
	}
}
