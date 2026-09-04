package orchestration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

const (
	FailureProvider        FailureKind = "provider"
	FailureNetwork         FailureKind = "network"
	FailureAuth            FailureKind = "auth"
	FailurePolicy          FailureKind = "policy"
	FailurePatchStale      FailureKind = "patch_stale"
	FailureConflict        FailureKind = "conflict"
	FailureContextOverflow FailureKind = "context_overflow"
)

type RecoveryAction string

const (
	RecoveryRepairCode     RecoveryAction = "repair_code"
	RecoveryReobserve      RecoveryAction = "reobserve"
	RecoveryCompactContext RecoveryAction = "compact_context"
	RecoveryReconcilePatch RecoveryAction = "reconcile_patch"
	RecoveryRetryProvider  RecoveryAction = "retry_provider"
	RecoverySwitchEngine   RecoveryAction = "switch_engine"
	RecoveryRequestUser    RecoveryAction = "request_user"
	RecoveryEscalate       RecoveryAction = "escalate"

	eventFailureDecision = "failure.decision"
)

var ErrInvalidFailureObservation = errors.New("invalid failure observation")

type FailureObservation struct {
	OccurrenceID string `json:"occurrenceId"`
	AgentID      string `json:"agentId,omitempty"`
	Source       string `json:"source,omitempty"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"-"`
}

type FailureAssessment struct {
	Signature    string         `json:"signature"`
	Kind         FailureKind    `json:"kind"`
	Action       RecoveryAction `json:"action"`
	MaxAttempts  int            `json:"maxAttempts"`
	Retryable    bool           `json:"retryable"`
	Reobserve    bool           `json:"reobserve,omitempty"`
	SwitchEngine bool           `json:"switchEngine,omitempty"`
	RequiresUser bool           `json:"requiresUser,omitempty"`
	ReasonCode   string         `json:"reasonCode"`
}

type RecoveryDecision struct {
	OccurrenceID string `json:"occurrenceId"`
	AgentID      string `json:"agentId,omitempty"`
	Attempt      int    `json:"attempt"`
	FailureAssessment
}

type RecoveryLedger struct {
	mu           sync.Mutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	attempts     map[string]int
	occurrences  map[string]RecoveryDecision
}

func NewRecoveryLedger(events *runtimeevents.Store, workspaceKey, taskID string) (*RecoveryLedger, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidFailureObservation
	}
	ledger := &RecoveryLedger{events: events, workspaceKey: workspaceKey, taskID: taskID, attempts: map[string]int{}, occurrences: map[string]RecoveryDecision{}}
	if events == nil {
		return ledger, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		if event.Type != eventFailureDecision {
			continue
		}
		var decision RecoveryDecision
		if err := failureDecode(event.Payload["decision"], &decision); err != nil {
			return nil, err
		}
		ledger.occurrences[decision.OccurrenceID] = decision
		if decision.Attempt > ledger.attempts[decision.Signature] {
			ledger.attempts[decision.Signature] = decision.Attempt
		}
	}
	return ledger, nil
}

// AnalyzeFailure turns raw failure text into a secret-safe assessment. The raw
// message participates only in a one-way signature and is never returned or
// persisted by the recovery ledger.
func AnalyzeFailure(observation FailureObservation) FailureAssessment {
	source := strings.ToLower(strings.TrimSpace(observation.Source))
	code := strings.ToLower(strings.TrimSpace(observation.Code))
	message := strings.ToLower(strings.TrimSpace(observation.Message))
	kind, action, maxAttempts, reason := classifyFailureSignal(source, code, message)
	assessment := FailureAssessment{Signature: failureSignature(source, code, message), Kind: kind, Action: action, MaxAttempts: maxAttempts, Retryable: maxAttempts > 0, ReasonCode: reason}
	switch action {
	case RecoveryReobserve:
		assessment.Reobserve = true
	case RecoverySwitchEngine:
		assessment.SwitchEngine = true
	case RecoveryRequestUser:
		assessment.RequiresUser = true
		assessment.Retryable = false
	}
	return assessment
}

func (l *RecoveryLedger) Decide(observation FailureObservation) (RecoveryDecision, error) {
	observation.OccurrenceID = strings.TrimSpace(observation.OccurrenceID)
	observation.AgentID = strings.TrimSpace(observation.AgentID)
	if observation.OccurrenceID == "" {
		return RecoveryDecision{}, ErrInvalidFailureObservation
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.occurrences[observation.OccurrenceID]; ok {
		return existing, nil
	}
	assessment := AnalyzeFailure(observation)
	attempt := l.attempts[assessment.Signature] + 1
	decision := RecoveryDecision{OccurrenceID: observation.OccurrenceID, AgentID: observation.AgentID, Attempt: attempt, FailureAssessment: assessment}
	if assessment.MaxAttempts == 0 {
		decision.Retryable = false
		decision.Reobserve = false
		decision.SwitchEngine = false
	} else if attempt > assessment.MaxAttempts {
		decision.Retryable = false
		decision.Reobserve = false
		decision.SwitchEngine = false
		if !decision.RequiresUser {
			decision.Action = RecoveryEscalate
			decision.ReasonCode = "retry_budget_exhausted"
		}
	}
	if l.events != nil {
		stored, appended, err := l.events.Append(l.workspaceKey, l.taskID, runtimeevents.Event{Type: eventFailureDecision, TaskID: l.taskID, AgentID: decision.AgentID, IdempotencyKey: "failure-decision:" + decision.OccurrenceID, Payload: map[string]any{"decision": failurePayload(decision)}})
		if err != nil {
			return RecoveryDecision{}, err
		}
		if !appended {
			var existing RecoveryDecision
			if err := failureDecode(stored.Payload["decision"], &existing); err != nil {
				return RecoveryDecision{}, err
			}
			decision = existing
		}
	}
	l.occurrences[decision.OccurrenceID] = decision
	if decision.Attempt > l.attempts[decision.Signature] {
		l.attempts[decision.Signature] = decision.Attempt
	}
	return decision, nil
}

func (l *RecoveryLedger) Attempts() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]int, len(l.attempts))
	for signature, count := range l.attempts {
		out[signature] = count
	}
	return out
}

func (l *RecoveryLedger) Decisions() []RecoveryDecision {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]RecoveryDecision, 0, len(l.occurrences))
	for _, decision := range l.occurrences {
		out = append(out, decision)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurrenceID < out[j].OccurrenceID })
	return out
}

func classifyFailureSignal(source, code, message string) (FailureKind, RecoveryAction, int, string) {
	text := strings.Join([]string{source, code, message}, " ")
	switch {
	case failureContains(code, "policy_denied", "approval_required") || failureContains(text, "policy denied", "approval required", "permission denied", "not authorized"):
		return FailurePolicy, RecoveryRequestUser, 0, "policy_or_approval_required"
	case failureContains(code, "auth_failed", "invalid_api_key", "unauthorized") || failureContains(text, "invalid api key", "authentication failed", "unauthorized"):
		return FailureAuth, RecoveryRequestUser, 0, "authentication_requires_user_action"
	case failureContains(code, "patch_stale", "stale_base", "hash_mismatch") || failureContains(text, "stale base", "expected hash mismatch", "file changed since"):
		return FailurePatchStale, RecoveryReconcilePatch, 2, "patch_base_changed"
	case failureContains(code, "patch_conflict", "merge_conflict", "conflict") || failureContains(text, "merge conflict", "patch conflict"):
		return FailureConflict, RecoveryReconcilePatch, 2, "concurrent_change_conflict"
	case failureContains(code, "context_overflow", "context_length_exceeded") || failureContains(text, "context length", "context window", "too many tokens"):
		return FailureContextOverflow, RecoveryCompactContext, 1, "context_budget_exceeded"
	case failureContains(code, "rate_limit", "quota_exceeded", "provider_unavailable") || failureContains(text, "rate limit", "quota exceeded", "provider unavailable", "too many requests"):
		return FailureProvider, RecoverySwitchEngine, 2, "provider_capacity_failure"
	case failureContains(code, "network", "timeout", "connection_reset") || failureContains(text, "connection reset", "temporary network", "timeout", "deadline exceeded"):
		return FailureNetwork, RecoveryRetryProvider, 2, "transient_network_failure"
	case failureContains(code, "stale_ui") || failureContains(text, "stale element", "detached from document", "window not found", "target closed"):
		return FailureStaleUI, RecoveryReobserve, 2, "ui_identity_changed"
	case failureContains(code, "compile") || failureContains(text, "syntax error", "undefined:", "build failed", "cannot find package"):
		return FailureCompile, RecoveryRepairCode, 2, "compile_diagnostic"
	case failureContains(code, "type") || failureContains(text, "type mismatch", "is not assignable", "ts2322", "ts2345", "cannot use"):
		return FailureType, RecoveryRepairCode, 2, "type_diagnostic"
	case failureContains(code, "test") || failureContains(text, "test failed", "--- fail:", "assertion failed"):
		return FailureTest, RecoveryRepairCode, 2, "verification_test_failure"
	case failureContains(code, "runtime") || failureContains(text, "panic:", "segmentation fault", "runtime error", "uncaught exception"):
		return FailureRuntime, RecoveryRepairCode, 1, "runtime_failure"
	case failureContains(code, "environment", "oom", "disk_full") || failureContains(text, "out of memory", "no space left", "disk quota", "cannot allocate memory"):
		return FailureEnvironment, RecoveryEscalate, 0, "environment_requires_external_change"
	default:
		return FailureUnknown, RecoveryEscalate, 0, "unclassified_failure"
	}
}

func failureContains(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func failureSignature(source, code, message string) string {
	normalized := strings.Join(strings.Fields(strings.Join([]string{source, code, message}, " ")), " ")
	sum := sha256.Sum256([]byte(normalized))
	return "fail_" + hex.EncodeToString(sum[:8])
}

func failurePayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func failureDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidFailureObservation
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidFailureObservation
	}
	return nil
}
