package orchestration

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

type VerificationStatus string

const (
	VerificationPending VerificationStatus = "pending"
	VerificationRunning VerificationStatus = "running"
	VerificationPassed  VerificationStatus = "passed"
	VerificationFailed  VerificationStatus = "failed"
	VerificationSkipped VerificationStatus = "skipped"

	eventVerificationPlan   = "verification.plan_created"
	eventVerificationResult = "verification.result_recorded"
)

var (
	ErrInvalidVerification      = errors.New("invalid verification state")
	ErrVerificationCheckMissing = errors.New("verification check not found")
	ErrStaleVerification        = errors.New("stale verification revision")
)

// VerificationRequirement is the durable, secret-safe form of a planned check.
// It deliberately excludes the raw command; command-specific identity is carried
// by CheckID so runtime truth does not need to persist a possibly sensitive CLI.
type VerificationRequirement struct {
	ID       string `json:"id"`
	Category string `json:"category,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Required bool   `json:"required"`
	Reason   string `json:"reason,omitempty"`
}

type VerificationEvidence struct {
	CheckID          string             `json:"checkId"`
	Status           VerificationStatus `json:"status"`
	AgentID          string             `json:"agentId,omitempty"`
	ExitCode         *int               `json:"exitCode,omitempty"`
	ArtifactRef      string             `json:"artifactRef,omitempty"`
	FailureSignature string             `json:"failureSignature,omitempty"`
	StartedAt        time.Time          `json:"startedAt,omitempty"`
	FinishedAt       time.Time          `json:"finishedAt,omitempty"`
}

type VerificationSnapshot struct {
	Revision     uint64                    `json:"revision"`
	Requirements []VerificationRequirement `json:"requirements"`
	Results      []VerificationEvidence    `json:"results,omitempty"`
}

type VerificationGate struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	revision     uint64
	requirements map[string]VerificationRequirement
	results      map[string]VerificationEvidence
}

func NewVerificationGate(events *runtimeevents.Store, workspaceKey, taskID string, plan VerificationPlan) (*VerificationGate, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidVerification
	}
	gate := &VerificationGate{
		events:       events,
		workspaceKey: workspaceKey,
		taskID:       taskID,
		requirements: map[string]VerificationRequirement{},
		results:      map[string]VerificationEvidence{},
	}
	if events != nil {
		stored, err := events.List(workspaceKey, taskID, 0, 5000)
		if err != nil {
			return nil, err
		}
		foundPlan := false
		for _, event := range stored {
			switch event.Type {
			case eventVerificationPlan:
				var snapshot VerificationSnapshot
				if err := verificationDecode(event.Payload["verification"], &snapshot); err != nil {
					return nil, err
				}
				gate.loadSnapshot(snapshot)
				foundPlan = true
			case eventVerificationResult:
				var snapshot VerificationSnapshot
				if err := verificationDecode(event.Payload["verification"], &snapshot); err != nil {
					return nil, err
				}
				gate.loadSnapshot(snapshot)
			}
		}
		if foundPlan {
			return gate, nil
		}
	}
	requirements, err := requirementsFromPlan(plan)
	if err != nil {
		return nil, err
	}
	gate.revision = 1
	for _, requirement := range requirements {
		gate.requirements[requirement.ID] = requirement
	}
	if err := gate.persistLocked(eventVerificationPlan, "verification:plan:1"); err != nil {
		return nil, err
	}
	return gate, nil
}

func (g *VerificationGate) Record(expectedRevision uint64, evidence VerificationEvidence) (VerificationSnapshot, error) {
	evidence = normalizeVerificationEvidence(evidence)
	g.mu.Lock()
	defer g.mu.Unlock()
	if expectedRevision == 0 || expectedRevision != g.revision {
		return VerificationSnapshot{}, ErrStaleVerification
	}
	if _, ok := g.requirements[evidence.CheckID]; !ok {
		return VerificationSnapshot{}, ErrVerificationCheckMissing
	}
	if !validVerificationStatus(evidence.Status) {
		return VerificationSnapshot{}, ErrInvalidVerification
	}
	if previous, ok := g.results[evidence.CheckID]; ok && !verificationTransitionAllowed(previous.Status, evidence.Status) {
		return VerificationSnapshot{}, ErrInvalidVerification
	}
	previous, hadPrevious := g.results[evidence.CheckID]
	previousRevision := g.revision
	g.results[evidence.CheckID] = evidence
	g.revision++
	if err := g.persistLocked(eventVerificationResult, "verification:result:"+evidence.CheckID+":"+strconv.FormatUint(g.revision, 10)); err != nil {
		g.revision = previousRevision
		if hadPrevious {
			g.results[evidence.CheckID] = previous
		} else {
			delete(g.results, evidence.CheckID)
		}
		return VerificationSnapshot{}, err
	}
	return g.snapshotLocked(), nil
}

func (g *VerificationGate) Snapshot() VerificationSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.snapshotLocked()
}

func (g *VerificationGate) CanComplete() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for id, requirement := range g.requirements {
		if !requirement.Required {
			continue
		}
		result, ok := g.results[id]
		if !ok || result.Status != VerificationPassed {
			return false
		}
	}
	return true
}

func (g *VerificationGate) FailedRequired() []VerificationEvidence {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := []VerificationEvidence{}
	for id, requirement := range g.requirements {
		if !requirement.Required {
			continue
		}
		if result, ok := g.results[id]; ok && result.Status == VerificationFailed {
			out = append(out, result)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CheckID < out[j].CheckID })
	return out
}

func (g *VerificationGate) persistLocked(eventType, idempotencyKey string) error {
	if g.events == nil {
		return nil
	}
	snapshot := g.snapshotLocked()
	_, _, err := g.events.Append(g.workspaceKey, g.taskID, runtimeevents.Event{
		Type:           eventType,
		TaskID:         g.taskID,
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"verification": verificationPayload(snapshot)},
	})
	return err
}

func (g *VerificationGate) loadSnapshot(snapshot VerificationSnapshot) {
	g.revision = snapshot.Revision
	g.requirements = map[string]VerificationRequirement{}
	g.results = map[string]VerificationEvidence{}
	for _, requirement := range snapshot.Requirements {
		g.requirements[requirement.ID] = requirement
	}
	for _, result := range snapshot.Results {
		g.results[result.CheckID] = result
	}
}

func (g *VerificationGate) snapshotLocked() VerificationSnapshot {
	requirements := make([]VerificationRequirement, 0, len(g.requirements))
	for _, requirement := range g.requirements {
		requirements = append(requirements, requirement)
	}
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].ID < requirements[j].ID })
	results := make([]VerificationEvidence, 0, len(g.results))
	for _, result := range g.results {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].CheckID < results[j].CheckID })
	return VerificationSnapshot{Revision: g.revision, Requirements: requirements, Results: results}
}

func requirementsFromPlan(plan VerificationPlan) ([]VerificationRequirement, error) {
	seen := map[string]struct{}{}
	out := make([]VerificationRequirement, 0, len(plan.Checks))
	for _, check := range plan.Checks {
		id := ScopedCheckID(check.Command, check.CWD)
		if id == "" {
			id = strings.TrimSpace(check.Key)
		}
		if id == "" {
			return nil, ErrInvalidVerification
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, VerificationRequirement{
			ID:       id,
			Category: CheckKey(check.Command),
			Scope:    strings.TrimSpace(check.Scope),
			Required: check.Required,
			Reason:   strings.TrimSpace(check.Reason),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func normalizeVerificationEvidence(evidence VerificationEvidence) VerificationEvidence {
	evidence.CheckID = strings.TrimSpace(evidence.CheckID)
	evidence.AgentID = strings.TrimSpace(evidence.AgentID)
	evidence.ArtifactRef = strings.TrimSpace(evidence.ArtifactRef)
	evidence.FailureSignature = strings.TrimSpace(evidence.FailureSignature)
	if !evidence.StartedAt.IsZero() {
		evidence.StartedAt = evidence.StartedAt.UTC()
	}
	if !evidence.FinishedAt.IsZero() {
		evidence.FinishedAt = evidence.FinishedAt.UTC()
	}
	return evidence
}

func validVerificationStatus(status VerificationStatus) bool {
	switch status {
	case VerificationPending, VerificationRunning, VerificationPassed, VerificationFailed, VerificationSkipped:
		return true
	default:
		return false
	}
}

func verificationTransitionAllowed(from, to VerificationStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case VerificationPending:
		return to == VerificationRunning || to == VerificationPassed || to == VerificationFailed || to == VerificationSkipped
	case VerificationRunning:
		return to == VerificationPassed || to == VerificationFailed || to == VerificationSkipped
	case VerificationFailed:
		return to == VerificationRunning || to == VerificationPassed
	case VerificationPassed, VerificationSkipped:
		return false
	default:
		return false
	}
}

func verificationPayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func verificationDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidVerification
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidVerification
	}
	return nil
}
