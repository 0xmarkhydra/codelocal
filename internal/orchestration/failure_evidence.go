package orchestration

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

const eventFailureEvidence = "failure.evidence_recorded"

var ErrInvalidFailureEvidence = errors.New("invalid failure evidence")

type FailureEvidenceBundle struct {
	OccurrenceID       string    `json:"occurrenceId"`
	AgentID            string    `json:"agentId,omitempty"`
	NodeID             string    `json:"nodeId,omitempty"`
	KernelRevision     uint64    `json:"kernelRevision,omitempty"`
	KernelStage        string    `json:"kernelStage,omitempty"`
	ContextFingerprint string    `json:"contextFingerprint,omitempty"`
	ToolRefs           []string  `json:"toolRefs,omitempty"`
	PatchRefs          []string  `json:"patchRefs,omitempty"`
	VerificationRefs   []string  `json:"verificationRefs,omitempty"`
	DecisionRefs       []string  `json:"decisionRefs,omitempty"`
	LastMutationRef    string    `json:"lastMutationRef,omitempty"`
	FailureSignature   string    `json:"failureSignature"`
	CapturedAt         time.Time `json:"capturedAt"`
}

type FailureAttribution struct {
	OccurrenceID   string  `json:"occurrenceId"`
	AgentID        string  `json:"agentId,omitempty"`
	ResponsibleRef string  `json:"responsibleRef,omitempty"`
	Confidence     float64 `json:"confidence"`
	ReasonCode     string  `json:"reasonCode"`
}

type FailureEvidenceStore struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	bundles      map[string]FailureEvidenceBundle
}

func NewFailureEvidenceStore(events *runtimeevents.Store, workspaceKey, taskID string) (*FailureEvidenceStore, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidFailureEvidence
	}
	store := &FailureEvidenceStore{events: events, workspaceKey: workspaceKey, taskID: taskID, bundles: map[string]FailureEvidenceBundle{}}
	if events == nil {
		return store, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		if event.Type != eventFailureEvidence {
			continue
		}
		var bundle FailureEvidenceBundle
		if err := failureEvidenceDecode(event.Payload["evidence"], &bundle); err != nil {
			return nil, err
		}
		bundle, err = normalizeFailureEvidence(bundle)
		if err != nil {
			return nil, err
		}
		store.bundles[bundle.OccurrenceID] = bundle
	}
	return store, nil
}

func (s *FailureEvidenceStore) Record(input FailureEvidenceBundle) (FailureEvidenceBundle, error) {
	bundle, err := normalizeFailureEvidence(input)
	if err != nil {
		return FailureEvidenceBundle{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bundles[bundle.OccurrenceID]; ok {
		return existing, nil
	}
	if s.events != nil {
		stored, appended, err := s.events.Append(s.workspaceKey, s.taskID, runtimeevents.Event{
			Type: eventFailureEvidence, TaskID: s.taskID, AgentID: bundle.AgentID,
			IdempotencyKey: "failure-evidence:" + bundle.OccurrenceID,
			Payload:        map[string]any{"evidence": failureEvidencePayload(bundle)},
		})
		if err != nil {
			return FailureEvidenceBundle{}, err
		}
		if !appended {
			var existing FailureEvidenceBundle
			if err := failureEvidenceDecode(stored.Payload["evidence"], &existing); err != nil {
				return FailureEvidenceBundle{}, err
			}
			bundle, err = normalizeFailureEvidence(existing)
			if err != nil {
				return FailureEvidenceBundle{}, err
			}
		}
	}
	s.bundles[bundle.OccurrenceID] = bundle
	return bundle, nil
}

func (s *FailureEvidenceStore) Get(occurrenceID string) (FailureEvidenceBundle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.bundles[strings.TrimSpace(occurrenceID)]
	return bundle, ok
}

func (s *FailureEvidenceStore) List() []FailureEvidenceBundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FailureEvidenceBundle, 0, len(s.bundles))
	for _, bundle := range s.bundles {
		out = append(out, bundle)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurrenceID < out[j].OccurrenceID })
	return out
}

// AttributeFailure is deliberately deterministic and cheap. It prefers an
// explicit last mutation reference, then verification evidence, then the owning
// agent. A later judge may refine this, but core retries never require an LLM to
// establish basic causal provenance.
func AttributeFailure(bundle FailureEvidenceBundle) FailureAttribution {
	attribution := FailureAttribution{OccurrenceID: bundle.OccurrenceID, AgentID: bundle.AgentID, Confidence: .35, ReasonCode: "owning_agent"}
	if bundle.LastMutationRef != "" {
		attribution.ResponsibleRef = bundle.LastMutationRef
		attribution.Confidence = .85
		attribution.ReasonCode = "last_mutation_before_failure"
		return attribution
	}
	if len(bundle.PatchRefs) > 0 {
		attribution.ResponsibleRef = bundle.PatchRefs[len(bundle.PatchRefs)-1]
		attribution.Confidence = .75
		attribution.ReasonCode = "latest_patch_before_failure"
		return attribution
	}
	if len(bundle.VerificationRefs) > 0 {
		attribution.ResponsibleRef = bundle.VerificationRefs[len(bundle.VerificationRefs)-1]
		attribution.Confidence = .6
		attribution.ReasonCode = "verification_failure_evidence"
	}
	return attribution
}

func normalizeFailureEvidence(bundle FailureEvidenceBundle) (FailureEvidenceBundle, error) {
	bundle.OccurrenceID = strings.TrimSpace(bundle.OccurrenceID)
	bundle.AgentID = strings.TrimSpace(bundle.AgentID)
	bundle.NodeID = strings.TrimSpace(bundle.NodeID)
	bundle.KernelStage = strings.ToLower(strings.TrimSpace(bundle.KernelStage))
	bundle.ContextFingerprint = strings.TrimSpace(bundle.ContextFingerprint)
	bundle.LastMutationRef = strings.TrimSpace(bundle.LastMutationRef)
	bundle.FailureSignature = strings.TrimSpace(bundle.FailureSignature)
	bundle.ToolRefs = normalizeEvidenceRefs(bundle.ToolRefs)
	bundle.PatchRefs = normalizeEvidenceRefs(bundle.PatchRefs)
	bundle.VerificationRefs = normalizeEvidenceRefs(bundle.VerificationRefs)
	bundle.DecisionRefs = normalizeEvidenceRefs(bundle.DecisionRefs)
	if bundle.OccurrenceID == "" || bundle.FailureSignature == "" {
		return FailureEvidenceBundle{}, ErrInvalidFailureEvidence
	}
	if bundle.CapturedAt.IsZero() {
		bundle.CapturedAt = time.Now().UTC()
	} else {
		bundle.CapturedAt = bundle.CapturedAt.UTC()
	}
	return bundle, nil
}

func normalizeEvidenceRefs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 256 {
			value = value[:256]
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func failureEvidencePayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func failureEvidenceDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidFailureEvidence
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidFailureEvidence
	}
	return nil
}
