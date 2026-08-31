package orchestration

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestRecoveryLedgerBoundsRepeatedRepairAndSurvivesRestart(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	ledger, err := NewRecoveryLedger(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	observations := []FailureObservation{
		{OccurrenceID: "f1", AgentID: "worker", Source: "verification", Code: "test", Message: "test failed: expected 2 got 1"},
		{OccurrenceID: "f2", AgentID: "worker", Source: "verification", Code: "test", Message: "test failed: expected 2 got 1"},
		{OccurrenceID: "f3", AgentID: "worker", Source: "verification", Code: "test", Message: "test failed: expected 2 got 1"},
	}
	for i, observation := range observations {
		decision, err := ledger.Decide(observation)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && (!decision.Retryable || decision.Action != RecoveryRepairCode) {
			t.Fatalf("attempt %d = %#v", i+1, decision)
		}
		if i == 2 && (decision.Retryable || decision.Action != RecoveryEscalate || decision.ReasonCode != "retry_budget_exhausted") {
			t.Fatalf("exhausted decision = %#v", decision)
		}
	}

	recovered, err := NewRecoveryLedger(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := recovered.Decide(FailureObservation{OccurrenceID: "f4", AgentID: "worker", Source: "verification", Code: "test", Message: "test failed: expected 2 got 1"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Attempt != 4 || decision.Retryable || decision.Action != RecoveryEscalate {
		t.Fatalf("recovered retry budget = %#v", decision)
	}
}

func TestRecoveryLedgerIsIdempotentAndDoesNotPersistRawFailure(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	ledger, err := NewRecoveryLedger(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	observation := FailureObservation{OccurrenceID: "same", AgentID: "worker", Source: "provider", Code: "rate_limit", Message: "rate limit secret-value"}
	first, err := ledger.Decide(observation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ledger.Decide(observation)
	if err != nil {
		t.Fatal(err)
	}
	if first.Attempt != second.Attempt || first.Signature != second.Signature {
		t.Fatalf("duplicate occurrence advanced budget: %#v %#v", first, second)
	}
	if first.Action != RecoverySwitchEngine || !first.SwitchEngine {
		t.Fatalf("provider decision = %#v", first)
	}
	events, err := store.List("workspace", "task", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "secret-value") {
		t.Fatalf("raw failure leaked into durable event: %s", raw)
	}
}

func TestAnalyzeFailureTargetsRecoveryDomain(t *testing.T) {
	cases := []struct {
		observation FailureObservation
		kind        FailureKind
		action      RecoveryAction
	}{
		{FailureObservation{Source: "patch", Code: "patch_stale", Message: "expected hash mismatch"}, FailurePatchStale, RecoveryReconcilePatch},
		{FailureObservation{Source: "model", Code: "context_overflow", Message: "context window exceeded"}, FailureContextOverflow, RecoveryCompactContext},
		{FailureObservation{Source: "computer", Code: "stale_ui", Message: "window not found"}, FailureStaleUI, RecoveryReobserve},
		{FailureObservation{Source: "policy", Code: "approval_required", Message: "approval required"}, FailurePolicy, RecoveryRequestUser},
	}
	for _, tc := range cases {
		assessment := AnalyzeFailure(tc.observation)
		if assessment.Kind != tc.kind || assessment.Action != tc.action {
			t.Fatalf("assessment for %#v = %#v", tc.observation, assessment)
		}
	}
}
