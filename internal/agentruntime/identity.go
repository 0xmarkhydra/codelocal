package agentruntime

import (
	"errors"
	"strings"
	"time"
)

type AgentRole string
type AgentStatus string
type ActivationStatus string

const (
	RoleLead         AgentRole = "lead"
	RoleInvestigator AgentRole = "investigator"
	RoleImplementer  AgentRole = "implementer"
	RoleReviewer     AgentRole = "reviewer"
	RoleTester       AgentRole = "tester"
	RoleSpecialist   AgentRole = "specialist"

	AgentCreated   AgentStatus = "created"
	AgentIdle      AgentStatus = "idle"
	AgentActive    AgentStatus = "active"
	AgentWaiting   AgentStatus = "waiting"
	AgentCompleted AgentStatus = "completed"
	AgentFailed    AgentStatus = "failed"
	AgentCancelled AgentStatus = "cancelled"

	ActivationStarting     ActivationStatus = "starting"
	ActivationRunning      ActivationStatus = "running"
	ActivationWaiting      ActivationStatus = "waiting"
	ActivationCheckpointed ActivationStatus = "checkpointed"
	ActivationStopped      ActivationStatus = "stopped"
	ActivationCompleted    ActivationStatus = "completed"
	ActivationFailed       ActivationStatus = "failed"
	ActivationCancelled    ActivationStatus = "cancelled"
	ActivationLost         ActivationStatus = "lost"
)

var (
	ErrInvalidAgentIdentity        = errors.New("invalid agent identity")
	ErrInvalidActivation           = errors.New("invalid agent activation")
	ErrInvalidAgentTransition      = errors.New("invalid agent status transition")
	ErrInvalidActivationTransition = errors.New("invalid activation status transition")
)

// AgentIdentity is the durable logical worker. It may survive process death,
// sandbox sleep, provider reconnects, and multiple live activations.
type AgentIdentity struct {
	ID            string      `json:"id"`
	TaskID        string      `json:"taskId"`
	ParentAgentID string      `json:"parentAgentId,omitempty"`
	Role          AgentRole   `json:"role"`
	EngineID      string      `json:"engineId"`
	ModelID       string      `json:"modelId,omitempty"`
	Status        AgentStatus `json:"status"`
	Revision      uint64      `json:"revision"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
}

// AgentActivation is one live attempt/runtime incarnation of an AgentIdentity.
// A continuable agent can have activation #1 terminate/checkpoint and later
// start activation #2 without losing its durable identity or task lineage.
type AgentActivation struct {
	ID                string           `json:"id"`
	AgentID           string           `json:"agentId"`
	Attempt           int              `json:"attempt"`
	RuntimeProvider   string           `json:"runtimeProvider,omitempty"`
	ProviderSessionID string           `json:"providerSessionId,omitempty"`
	ProcessID         string           `json:"processId,omitempty"`
	Status            ActivationStatus `json:"status"`
	Revision          uint64           `json:"revision"`
	StartedAt         time.Time        `json:"startedAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
	EndedAt           time.Time        `json:"endedAt,omitempty"`
}

func NormalizeIdentity(identity AgentIdentity) (AgentIdentity, error) {
	identity.ID = strings.TrimSpace(identity.ID)
	identity.TaskID = strings.TrimSpace(identity.TaskID)
	identity.ParentAgentID = strings.TrimSpace(identity.ParentAgentID)
	identity.EngineID = strings.ToLower(strings.TrimSpace(identity.EngineID))
	identity.ModelID = strings.TrimSpace(identity.ModelID)
	if identity.Role == "" {
		identity.Role = RoleImplementer
	}
	if identity.Status == "" {
		identity.Status = AgentCreated
	}
	if identity.ID == "" || identity.TaskID == "" || identity.EngineID == "" || !validAgentStatus(identity.Status) {
		return AgentIdentity{}, ErrInvalidAgentIdentity
	}
	now := time.Now().UTC()
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = now
	} else {
		identity.CreatedAt = identity.CreatedAt.UTC()
	}
	if identity.UpdatedAt.IsZero() {
		identity.UpdatedAt = identity.CreatedAt
	} else {
		identity.UpdatedAt = identity.UpdatedAt.UTC()
	}
	if identity.Revision == 0 {
		identity.Revision = 1
	}
	return identity, nil
}

func NormalizeActivation(activation AgentActivation) (AgentActivation, error) {
	activation.ID = strings.TrimSpace(activation.ID)
	activation.AgentID = strings.TrimSpace(activation.AgentID)
	activation.RuntimeProvider = strings.TrimSpace(activation.RuntimeProvider)
	activation.ProviderSessionID = strings.TrimSpace(activation.ProviderSessionID)
	activation.ProcessID = strings.TrimSpace(activation.ProcessID)
	if activation.Status == "" {
		activation.Status = ActivationStarting
	}
	if activation.Attempt <= 0 {
		activation.Attempt = 1
	}
	if activation.ID == "" || activation.AgentID == "" || !validActivationStatus(activation.Status) {
		return AgentActivation{}, ErrInvalidActivation
	}
	now := time.Now().UTC()
	if activation.StartedAt.IsZero() {
		activation.StartedAt = now
	} else {
		activation.StartedAt = activation.StartedAt.UTC()
	}
	if activation.UpdatedAt.IsZero() {
		activation.UpdatedAt = activation.StartedAt
	} else {
		activation.UpdatedAt = activation.UpdatedAt.UTC()
	}
	if activation.Revision == 0 {
		activation.Revision = 1
	}
	if activationTerminal(activation.Status) && activation.EndedAt.IsZero() {
		activation.EndedAt = activation.UpdatedAt
	} else if !activation.EndedAt.IsZero() {
		activation.EndedAt = activation.EndedAt.UTC()
	}
	return activation, nil
}

func TransitionIdentity(identity AgentIdentity, next AgentStatus, at time.Time) (AgentIdentity, error) {
	identity, err := NormalizeIdentity(identity)
	if err != nil {
		return AgentIdentity{}, err
	}
	if !agentTransitionAllowed(identity.Status, next) {
		return AgentIdentity{}, ErrInvalidAgentTransition
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	identity.Status = next
	identity.Revision++
	identity.UpdatedAt = at
	return identity, nil
}

func TransitionActivation(activation AgentActivation, next ActivationStatus, at time.Time) (AgentActivation, error) {
	activation, err := NormalizeActivation(activation)
	if err != nil {
		return AgentActivation{}, err
	}
	if !activationTransitionAllowed(activation.Status, next) {
		return AgentActivation{}, ErrInvalidActivationTransition
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	activation.Status = next
	activation.Revision++
	activation.UpdatedAt = at
	if activationTerminal(next) {
		activation.EndedAt = at
	}
	return activation, nil
}

func validAgentStatus(status AgentStatus) bool {
	switch status {
	case AgentCreated, AgentIdle, AgentActive, AgentWaiting, AgentCompleted, AgentFailed, AgentCancelled:
		return true
	default:
		return false
	}
}

func validActivationStatus(status ActivationStatus) bool {
	switch status {
	case ActivationStarting, ActivationRunning, ActivationWaiting, ActivationCheckpointed, ActivationStopped, ActivationCompleted, ActivationFailed, ActivationCancelled, ActivationLost:
		return true
	default:
		return false
	}
}

func agentTransitionAllowed(from, to AgentStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case AgentCreated:
		return to == AgentActive || to == AgentIdle || to == AgentCancelled || to == AgentFailed
	case AgentIdle:
		return to == AgentActive || to == AgentCancelled || to == AgentCompleted || to == AgentFailed
	case AgentActive:
		return to == AgentWaiting || to == AgentIdle || to == AgentCompleted || to == AgentFailed || to == AgentCancelled
	case AgentWaiting:
		return to == AgentActive || to == AgentIdle || to == AgentCancelled || to == AgentFailed
	case AgentCompleted, AgentFailed, AgentCancelled:
		return false
	default:
		return false
	}
}

func activationTerminal(status ActivationStatus) bool {
	switch status {
	case ActivationCheckpointed, ActivationStopped, ActivationCompleted, ActivationFailed, ActivationCancelled, ActivationLost:
		return true
	default:
		return false
	}
}

func activationTransitionAllowed(from, to ActivationStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case ActivationStarting:
		return to == ActivationRunning || to == ActivationCheckpointed || to == ActivationStopped || to == ActivationFailed || to == ActivationCancelled || to == ActivationLost
	case ActivationRunning:
		return to == ActivationWaiting || to == ActivationCheckpointed || to == ActivationStopped || to == ActivationCompleted || to == ActivationFailed || to == ActivationCancelled || to == ActivationLost
	case ActivationWaiting:
		return to == ActivationRunning || to == ActivationCheckpointed || to == ActivationStopped || to == ActivationCompleted || to == ActivationFailed || to == ActivationCancelled || to == ActivationLost
	case ActivationCheckpointed, ActivationStopped, ActivationCompleted, ActivationFailed, ActivationCancelled, ActivationLost:
		return false
	default:
		return false
	}
}
