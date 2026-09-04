package agentruntime

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

const (
	eventActivationStarted      = "activation.started"
	eventActivationTransitioned = "activation.transitioned"
)

var (
	ErrInvalidActivationManager = errors.New("invalid activation manager")
	ErrActivationNotFound       = errors.New("activation not found")
	ErrActivationBusy           = errors.New("agent already has a live activation")
	ErrStaleActivationRevision  = errors.New("stale activation revision")
	ErrActivationAgentTerminal  = errors.New("terminal agent cannot start activation")
)

type ActivationStart struct {
	ID                string `json:"id,omitempty"`
	AgentID           string `json:"agentId"`
	RuntimeProvider   string `json:"runtimeProvider"`
	ProviderSessionID string `json:"providerSessionId,omitempty"`
	ProcessID         string `json:"processId,omitempty"`
}

// ActivationManager owns disposable runtime incarnations while AgentGraph owns
// durable logical identities. Every transition is appended before hot state is
// changed so restart can reconstruct attempts and fencing revisions.
type ActivationManager struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	graph        *AgentGraph
	activations  map[string]AgentActivation
	latest       map[string]string
	attempts     map[string]int
}

func NewActivationManager(events *runtimeevents.Store, workspaceKey, taskID string, graph *AgentGraph) (*ActivationManager, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" || graph == nil || graph.taskID != taskID {
		return nil, ErrInvalidActivationManager
	}
	manager := &ActivationManager{
		events:       events,
		workspaceKey: workspaceKey,
		taskID:       taskID,
		graph:        graph,
		activations:  map[string]AgentActivation{},
		latest:       map[string]string{},
		attempts:     map[string]int{},
	}
	if events == nil {
		return manager, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		if event.Type != eventActivationStarted && event.Type != eventActivationTransitioned {
			continue
		}
		var activation AgentActivation
		if err := graphDecode(event.Payload["activation"], &activation); err != nil || !validPersistedActivation(activation) {
			return nil, ErrInvalidActivationManager
		}
		if err := manager.applyReplayLocked(event.Type, activation); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func (m *ActivationManager) Start(input ActivationStart) (AgentActivation, error) {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.ID = strings.TrimSpace(input.ID)
	input.RuntimeProvider = strings.TrimSpace(input.RuntimeProvider)
	input.ProviderSessionID = strings.TrimSpace(input.ProviderSessionID)
	input.ProcessID = strings.TrimSpace(input.ProcessID)
	if input.AgentID == "" || input.RuntimeProvider == "" {
		return AgentActivation{}, ErrInvalidActivation
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	agent, ok := m.graph.Agent(input.AgentID)
	if !ok {
		return AgentActivation{}, ErrAgentNotFound
	}
	if agentTerminal(agent.Status) {
		return AgentActivation{}, ErrActivationAgentTerminal
	}
	if latestID := m.latest[input.AgentID]; latestID != "" {
		latest := m.activations[latestID]
		if !activationTerminal(latest.Status) {
			return AgentActivation{}, ErrActivationBusy
		}
	}
	attempt := m.attempts[input.AgentID] + 1
	if input.ID == "" {
		input.ID = "activation:" + input.AgentID + ":" + strconv.Itoa(attempt)
	}
	if _, exists := m.activations[input.ID]; exists {
		return AgentActivation{}, ErrActivationBusy
	}
	activation, err := NormalizeActivation(AgentActivation{
		ID:                input.ID,
		AgentID:           input.AgentID,
		Attempt:           attempt,
		RuntimeProvider:   input.RuntimeProvider,
		ProviderSessionID: input.ProviderSessionID,
		ProcessID:         input.ProcessID,
		Status:            ActivationStarting,
	})
	if err != nil {
		return AgentActivation{}, err
	}
	if err := m.appendLocked(eventActivationStarted, activation, "activation:start:"+activation.ID); err != nil {
		return AgentActivation{}, err
	}
	m.activations[activation.ID] = activation
	m.latest[activation.AgentID] = activation.ID
	m.attempts[activation.AgentID] = activation.Attempt
	return activation, nil
}

func (m *ActivationManager) TransitionCAS(activationID string, expectedRevision uint64, next ActivationStatus, at time.Time) (AgentActivation, error) {
	activationID = strings.TrimSpace(activationID)
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.activations[activationID]
	if !ok {
		return AgentActivation{}, ErrActivationNotFound
	}
	if expectedRevision == 0 || expectedRevision != current.Revision {
		return AgentActivation{}, ErrStaleActivationRevision
	}
	updated, err := TransitionActivation(current, next, at)
	if err != nil {
		return AgentActivation{}, err
	}
	key := "activation:transition:" + activationID + ":" + strconv.FormatUint(updated.Revision, 10)
	if err := m.appendLocked(eventActivationTransitioned, updated, key); err != nil {
		return AgentActivation{}, err
	}
	m.activations[activationID] = updated
	m.latest[updated.AgentID] = updated.ID
	return updated, nil
}

func (m *ActivationManager) MarkLost(activationID string, expectedRevision uint64, at time.Time) (AgentActivation, error) {
	return m.TransitionCAS(activationID, expectedRevision, ActivationLost, at)
}

func (m *ActivationManager) Activation(activationID string) (AgentActivation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	activation, ok := m.activations[strings.TrimSpace(activationID)]
	return activation, ok
}

func (m *ActivationManager) Latest(agentID string) (AgentActivation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id := m.latest[strings.TrimSpace(agentID)]
	if id == "" {
		return AgentActivation{}, false
	}
	activation, ok := m.activations[id]
	return activation, ok
}

func (m *ActivationManager) ForAgent(agentID string) []AgentActivation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agentID = strings.TrimSpace(agentID)
	out := []AgentActivation{}
	for _, activation := range m.activations {
		if activation.AgentID == agentID {
			out = append(out, activation)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attempt != out[j].Attempt {
			return out[i].Attempt < out[j].Attempt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (m *ActivationManager) HasLive(agentID string) bool {
	activation, ok := m.Latest(agentID)
	return ok && !activationTerminal(activation.Status)
}

func (m *ActivationManager) appendLocked(eventType string, activation AgentActivation, idempotencyKey string) error {
	if m.events == nil {
		return nil
	}
	_, _, err := m.events.Append(m.workspaceKey, m.taskID, runtimeevents.Event{
		Type:           eventType,
		TaskID:         m.taskID,
		AgentID:        activation.AgentID,
		ActivationID:   activation.ID,
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"activation": graphPayload(activation)},
	})
	return err
}

func (m *ActivationManager) applyReplayLocked(eventType string, activation AgentActivation) error {
	agent, ok := m.graph.Agent(activation.AgentID)
	if !ok {
		return ErrInvalidActivationManager
	}
	if agent.TaskID != m.taskID {
		return ErrInvalidActivationManager
	}
	current, exists := m.activations[activation.ID]
	switch eventType {
	case eventActivationStarted:
		if exists || activation.Revision != 1 {
			return ErrInvalidActivationManager
		}
		if latestID := m.latest[activation.AgentID]; latestID != "" {
			latest := m.activations[latestID]
			if !activationTerminal(latest.Status) || activation.Attempt != latest.Attempt+1 {
				return ErrInvalidActivationManager
			}
		} else if activation.Attempt != 1 {
			return ErrInvalidActivationManager
		}
	case eventActivationTransitioned:
		if !exists || activation.AgentID != current.AgentID || activation.Attempt != current.Attempt || activation.Revision != current.Revision+1 {
			return ErrInvalidActivationManager
		}
		if !activationTransitionAllowed(current.Status, activation.Status) {
			return ErrInvalidActivationManager
		}
	default:
		return nil
	}
	m.activations[activation.ID] = activation
	m.latest[activation.AgentID] = activation.ID
	if activation.Attempt > m.attempts[activation.AgentID] {
		m.attempts[activation.AgentID] = activation.Attempt
	}
	return nil
}

func validPersistedActivation(activation AgentActivation) bool {
	if strings.TrimSpace(activation.ID) == "" || strings.TrimSpace(activation.AgentID) == "" || strings.TrimSpace(activation.RuntimeProvider) == "" {
		return false
	}
	if activation.Attempt <= 0 || activation.Revision == 0 || activation.StartedAt.IsZero() || activation.UpdatedAt.IsZero() || !validActivationStatus(activation.Status) {
		return false
	}
	if activationTerminal(activation.Status) && activation.EndedAt.IsZero() {
		return false
	}
	return true
}
