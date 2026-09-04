package orchestration

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

type KernelStage string

const (
	KernelUnderstand KernelStage = "understand"
	KernelLocate     KernelStage = "locate"
	KernelPlan       KernelStage = "plan"
	KernelAct        KernelStage = "act"
	KernelObserve    KernelStage = "observe"
	KernelVerify     KernelStage = "verify"
	KernelReflect    KernelStage = "reflect"
	KernelRepair     KernelStage = "repair"
	KernelBlocked    KernelStage = "blocked"
	KernelFailed     KernelStage = "failed"
	KernelComplete   KernelStage = "complete"

	eventKernelInitialized = "codingkernel.initialized"
	eventKernelTransition  = "codingkernel.transitioned"
	eventKernelFailure     = "codingkernel.failure_handled"
)

var (
	ErrInvalidCodingKernel       = errors.New("invalid coding kernel")
	ErrStaleKernelRevision       = errors.New("stale coding kernel revision")
	ErrInvalidKernelTransition   = errors.New("invalid coding kernel transition")
	ErrKernelCompletionBlocked   = errors.New("coding kernel completion is blocked")
	ErrKernelRecoveryUnavailable = errors.New("coding kernel recovery is unavailable")
)

type KernelSnapshot struct {
	Revision             uint64         `json:"revision"`
	Stage                KernelStage    `json:"stage"`
	Cycle                uint64         `json:"cycle"`
	LastOccurrenceID     string         `json:"lastOccurrenceId,omitempty"`
	LastFailureSignature string         `json:"lastFailureSignature,omitempty"`
	LastFailureStage     KernelStage    `json:"lastFailureStage,omitempty"`
	LastRecoveryAction   RecoveryAction `json:"lastRecoveryAction,omitempty"`
	LastReasonCode       string         `json:"lastReasonCode,omitempty"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

type CodingKernel struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	runtime      *ShadowRuntime
	state        KernelSnapshot
}

func NewCodingKernel(events *runtimeevents.Store, workspaceKey, taskID string, runtime *ShadowRuntime) (*CodingKernel, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" || runtime == nil || runtime.Recovery() == nil {
		return nil, ErrInvalidCodingKernel
	}
	kernel := &CodingKernel{events: events, workspaceKey: workspaceKey, taskID: taskID, runtime: runtime}
	if events != nil {
		stored, err := events.List(workspaceKey, taskID, 0, 5000)
		if err != nil {
			return nil, err
		}
		for _, event := range stored {
			if event.Type != eventKernelInitialized && event.Type != eventKernelTransition && event.Type != eventKernelFailure {
				continue
			}
			var snapshot KernelSnapshot
			if err := kernelDecode(event.Payload["kernel"], &snapshot); err != nil || !validKernelSnapshot(snapshot) {
				return nil, ErrInvalidCodingKernel
			}
			if kernel.state.Revision != 0 && snapshot.Revision <= kernel.state.Revision {
				return nil, ErrInvalidCodingKernel
			}
			kernel.state = snapshot
		}
		if kernel.state.Revision != 0 {
			return kernel, nil
		}
	}
	kernel.state = KernelSnapshot{Revision: 1, Stage: KernelUnderstand, UpdatedAt: time.Now().UTC()}
	if err := kernel.persistInitial(); err != nil {
		return nil, err
	}
	return kernel, nil
}

func (k *CodingKernel) Snapshot() KernelSnapshot {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.state
}

func (k *CodingKernel) Advance(expectedRevision uint64, next KernelStage, reasonCode string) (KernelSnapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if expectedRevision == 0 || expectedRevision != k.state.Revision {
		return KernelSnapshot{}, ErrStaleKernelRevision
	}
	if !normalKernelTransitionAllowed(k.state.Stage, next) {
		return KernelSnapshot{}, ErrInvalidKernelTransition
	}
	updated := k.state
	updated.Revision++
	updated.Stage = next
	updated.LastReasonCode = normalizeKernelReason(reasonCode)
	updated.UpdatedAt = time.Now().UTC()
	if err := k.persistSnapshot(eventKernelTransition, updated, "kernel:transition:"+strconv.FormatUint(updated.Revision, 10)); err != nil {
		return KernelSnapshot{}, err
	}
	k.state = updated
	return updated, nil
}

// HandleFailure consumes one durable recovery-budget occurrence only after the
// kernel revision/stage have been validated. Raw failure text is handled by the
// RecoveryLedger and never copied into the kernel event stream.
func (k *CodingKernel) HandleFailure(expectedRevision uint64, observation FailureObservation) (KernelSnapshot, RecoveryDecision, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if expectedRevision == 0 || expectedRevision != k.state.Revision {
		return KernelSnapshot{}, RecoveryDecision{}, ErrStaleKernelRevision
	}
	if !kernelFailureStage(k.state.Stage) {
		return KernelSnapshot{}, RecoveryDecision{}, ErrInvalidKernelTransition
	}
	interruptedStage := k.state.Stage
	decision, err := k.runtime.Recovery().Decide(observation)
	if err != nil {
		return KernelSnapshot{}, RecoveryDecision{}, err
	}
	updated := k.state
	updated.Revision++
	updated.LastOccurrenceID = decision.OccurrenceID
	updated.LastFailureSignature = decision.Signature
	updated.LastFailureStage = interruptedStage
	updated.LastRecoveryAction = decision.Action
	updated.LastReasonCode = normalizeKernelReason(decision.ReasonCode)
	updated.UpdatedAt = time.Now().UTC()
	switch {
	case decision.RequiresUser:
		updated.Stage = KernelBlocked
	case decision.Retryable:
		updated.Stage = KernelRepair
	default:
		updated.Stage = KernelFailed
	}
	key := "kernel:failure:" + decision.OccurrenceID + ":" + strconv.FormatUint(updated.Revision, 10)
	if err := k.persistSnapshot(eventKernelFailure, updated, key); err != nil {
		return KernelSnapshot{}, RecoveryDecision{}, err
	}
	k.state = updated
	return updated, decision, nil
}

func (k *CodingKernel) ResumeRecovery(expectedRevision uint64) (KernelSnapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if expectedRevision == 0 || expectedRevision != k.state.Revision {
		return KernelSnapshot{}, ErrStaleKernelRevision
	}
	if k.state.Stage != KernelRepair {
		return KernelSnapshot{}, ErrKernelRecoveryUnavailable
	}
	target := recoveryResumeStage(k.state.LastRecoveryAction, k.state.LastFailureStage)
	if target == "" {
		return KernelSnapshot{}, ErrKernelRecoveryUnavailable
	}
	updated := k.state
	updated.Revision++
	updated.Cycle++
	updated.Stage = target
	updated.LastReasonCode = "recovery_resumed"
	updated.UpdatedAt = time.Now().UTC()
	if err := k.persistSnapshot(eventKernelTransition, updated, "kernel:resume-recovery:"+strconv.FormatUint(updated.Revision, 10)); err != nil {
		return KernelSnapshot{}, err
	}
	k.state = updated
	return updated, nil
}

// ResumeBlocked is only for a user/policy/auth action that has been satisfied
// outside the kernel. It retries the interrupted stage without weakening the
// policy decision that caused the block.
func (k *CodingKernel) ResumeBlocked(expectedRevision uint64) (KernelSnapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if expectedRevision == 0 || expectedRevision != k.state.Revision {
		return KernelSnapshot{}, ErrStaleKernelRevision
	}
	if k.state.Stage != KernelBlocked || k.state.LastRecoveryAction != RecoveryRequestUser || !activeKernelStage(k.state.LastFailureStage) {
		return KernelSnapshot{}, ErrKernelRecoveryUnavailable
	}
	updated := k.state
	updated.Revision++
	updated.Cycle++
	updated.Stage = k.state.LastFailureStage
	updated.LastReasonCode = "user_action_satisfied"
	updated.UpdatedAt = time.Now().UTC()
	if err := k.persistSnapshot(eventKernelTransition, updated, "kernel:resume-blocked:"+strconv.FormatUint(updated.Revision, 10)); err != nil {
		return KernelSnapshot{}, err
	}
	k.state = updated
	return updated, nil
}

// Complete is the only transition into COMPLETE. The durable orchestration
// runtime must already prove DAG success, descendant-agent success and required
// verification PASS. Lead finalization happens first; if the kernel append then
// fails, retry is safe because lead finalization is idempotent.
func (k *CodingKernel) Complete(expectedRevision uint64, leadAgentID string, leadExpectedRevision uint64, at time.Time) (KernelSnapshot, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if expectedRevision == 0 || expectedRevision != k.state.Revision {
		return KernelSnapshot{}, ErrStaleKernelRevision
	}
	if k.state.Stage != KernelReflect || !k.runtime.ReadyToFinalize(leadAgentID) {
		return KernelSnapshot{}, ErrKernelCompletionBlocked
	}
	if _, err := k.runtime.FinalizeLead(leadAgentID, leadExpectedRevision, at); err != nil {
		return KernelSnapshot{}, err
	}
	updated := k.state
	updated.Revision++
	updated.Stage = KernelComplete
	updated.LastReasonCode = "verified_complete"
	updated.UpdatedAt = time.Now().UTC()
	if err := k.persistSnapshot(eventKernelTransition, updated, "kernel:complete:"+strconv.FormatUint(updated.Revision, 10)); err != nil {
		return KernelSnapshot{}, err
	}
	k.state = updated
	return updated, nil
}

func (k *CodingKernel) persistInitial() error {
	if k.events == nil {
		return nil
	}
	stored, appended, err := k.events.Append(k.workspaceKey, k.taskID, runtimeevents.Event{
		Type:           eventKernelInitialized,
		TaskID:         k.taskID,
		IdempotencyKey: "kernel:init",
		Payload:        map[string]any{"kernel": kernelPayload(k.state)},
	})
	if err != nil {
		return err
	}
	if appended {
		return nil
	}
	var existing KernelSnapshot
	if err := kernelDecode(stored.Payload["kernel"], &existing); err != nil || !validKernelSnapshot(existing) {
		return ErrInvalidCodingKernel
	}
	k.state = existing
	return nil
}

func (k *CodingKernel) persistSnapshot(eventType string, snapshot KernelSnapshot, idempotencyKey string) error {
	if k.events == nil {
		return nil
	}
	_, _, err := k.events.Append(k.workspaceKey, k.taskID, runtimeevents.Event{
		Type:           eventType,
		TaskID:         k.taskID,
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"kernel": kernelPayload(snapshot)},
	})
	return err
}

func normalKernelTransitionAllowed(from, to KernelStage) bool {
	switch from {
	case KernelUnderstand:
		return to == KernelLocate
	case KernelLocate:
		return to == KernelPlan
	case KernelPlan:
		return to == KernelAct
	case KernelAct:
		return to == KernelObserve
	case KernelObserve:
		return to == KernelVerify
	case KernelVerify:
		return to == KernelReflect
	default:
		return false
	}
}

func activeKernelStage(stage KernelStage) bool {
	switch stage {
	case KernelUnderstand, KernelLocate, KernelPlan, KernelAct, KernelObserve, KernelVerify, KernelReflect:
		return true
	default:
		return false
	}
}

func kernelFailureStage(stage KernelStage) bool {
	return activeKernelStage(stage)
}

func recoveryResumeStage(action RecoveryAction, interrupted KernelStage) KernelStage {
	switch action {
	case RecoveryReobserve:
		return KernelObserve
	case RecoveryCompactContext:
		return KernelPlan
	case RecoveryRepairCode, RecoveryReconcilePatch:
		return KernelAct
	case RecoveryRetryProvider, RecoverySwitchEngine:
		if activeKernelStage(interrupted) {
			return interrupted
		}
		return ""
	default:
		return ""
	}
}

func validKernelSnapshot(snapshot KernelSnapshot) bool {
	if snapshot.Revision == 0 || snapshot.UpdatedAt.IsZero() {
		return false
	}
	switch snapshot.Stage {
	case KernelUnderstand, KernelLocate, KernelPlan, KernelAct, KernelObserve, KernelVerify, KernelReflect, KernelRepair, KernelBlocked, KernelFailed, KernelComplete:
		return true
	default:
		return false
	}
}

func normalizeKernelReason(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "stage_advanced"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	return strings.Trim(b.String(), "_")
}

func kernelPayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func kernelDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidCodingKernel
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidCodingKernel
	}
	return nil
}
