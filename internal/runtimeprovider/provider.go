package runtimeprovider

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateCold         State = "cold"
	StateProvisioning State = "provisioning"
	StateActive       State = "active"
	StateIdle         State = "idle"
	StateCheckpointed State = "checkpointed"
	StateSleeping     State = "sleeping"
	StateStopped      State = "stopped"
	StateFailed       State = "failed"
)

var (
	ErrInvalidRuntimeProvider = errors.New("invalid runtime provider")
	ErrRuntimeProviderExists  = errors.New("runtime provider already registered")
	ErrRuntimeProviderMissing = errors.New("runtime provider not found")
	ErrRuntimeStateTransition = errors.New("invalid runtime provider state transition")
)

type Capabilities struct {
	FileSystem        bool `json:"fileSystem"`
	Shell             bool `json:"shell"`
	Network           bool `json:"network"`
	Browser           bool `json:"browser"`
	Computer          bool `json:"computer"`
	Mobile            bool `json:"mobile"`
	DurableCheckpoint bool `json:"durableCheckpoint"`
}

type Probe struct {
	ProviderID   string       `json:"providerId"`
	Available    bool         `json:"available"`
	Capabilities Capabilities `json:"capabilities"`
	Reason       string       `json:"reason,omitempty"`
	CheckedAt    time.Time    `json:"checkedAt"`
}

type Request struct {
	WorkspaceKey string       `json:"workspaceKey"`
	TaskID       string       `json:"taskId"`
	AgentID      string       `json:"agentId"`
	ActivationID string       `json:"activationId"`
	Required     Capabilities `json:"required"`
}

type Handle struct {
	ID           string       `json:"id"`
	ProviderID   string       `json:"providerId"`
	WorkspaceKey string       `json:"workspaceKey"`
	TaskID       string       `json:"taskId"`
	AgentID      string       `json:"agentId"`
	ActivationID string       `json:"activationId"`
	State        State        `json:"state"`
	Capabilities Capabilities `json:"capabilities"`
	CheckpointRef string      `json:"checkpointRef,omitempty"`
	Revision     uint64       `json:"revision"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

type Provider interface {
	ID() string
	Probe(context.Context) Probe
	Provision(context.Context, Request) (Handle, error)
	Checkpoint(context.Context, Handle) (Handle, error)
	Sleep(context.Context, Handle) (Handle, error)
	Resume(context.Context, Handle) (Handle, error)
	Stop(context.Context, Handle) (Handle, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: map[string]Provider{}} }

func (r *Registry) Register(provider Provider) error {
	if r == nil || provider == nil || strings.TrimSpace(provider.ID()) == "" { return ErrInvalidRuntimeProvider }
	id := strings.ToLower(strings.TrimSpace(provider.ID()))
	r.mu.Lock(); defer r.mu.Unlock()
	if _, exists := r.providers[id]; exists { return ErrRuntimeProviderExists }
	r.providers[id] = provider
	return nil
}

func (r *Registry) Get(id string) (Provider, bool) {
	if r == nil { return nil, false }
	r.mu.RLock(); defer r.mu.RUnlock()
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(id))]
	return provider, ok
}

func (r *Registry) IDs() []string {
	if r == nil { return nil }
	r.mu.RLock(); defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for id := range r.providers { out = append(out, id) }
	sort.Strings(out)
	return out
}

func Choose(probes []Probe, required Capabilities) (Probe, error) {
	viable := []Probe{}
	for _, probe := range probes {
		if !probe.Available || !supports(probe.Capabilities, required) { continue }
		viable = append(viable, probe)
	}
	if len(viable) == 0 { return Probe{}, ErrRuntimeProviderMissing }
	sort.SliceStable(viable, func(i, j int) bool {
		left, right := capabilityCount(viable[i].Capabilities), capabilityCount(viable[j].Capabilities)
		if left != right { return left < right }
		return viable[i].ProviderID < viable[j].ProviderID
	})
	return viable[0], nil
}

func Transition(handle Handle, expectedRevision uint64, next State, at time.Time) (Handle, error) {
	if handle.ID == "" || handle.ProviderID == "" || expectedRevision == 0 || expectedRevision != handle.Revision { return Handle{}, ErrRuntimeStateTransition }
	if !stateTransitionAllowed(handle.State, next) { return Handle{}, ErrRuntimeStateTransition }
	if at.IsZero() { at = time.Now().UTC() } else { at = at.UTC() }
	handle.State = next
	handle.Revision++
	handle.UpdatedAt = at
	return handle, nil
}

func supports(have, need Capabilities) bool {
	return (!need.FileSystem || have.FileSystem) && (!need.Shell || have.Shell) && (!need.Network || have.Network) && (!need.Browser || have.Browser) && (!need.Computer || have.Computer) && (!need.Mobile || have.Mobile) && (!need.DurableCheckpoint || have.DurableCheckpoint)
}

func capabilityCount(c Capabilities) int { count := 0; for _, v := range []bool{c.FileSystem,c.Shell,c.Network,c.Browser,c.Computer,c.Mobile,c.DurableCheckpoint} { if v { count++ } }; return count }

func stateTransitionAllowed(from, to State) bool {
	if from == to { return true }
	switch from {
	case StateCold: return to == StateProvisioning || to == StateFailed
	case StateProvisioning: return to == StateActive || to == StateFailed || to == StateStopped
	case StateActive: return to == StateIdle || to == StateCheckpointed || to == StateStopped || to == StateFailed
	case StateIdle: return to == StateActive || to == StateCheckpointed || to == StateSleeping || to == StateStopped || to == StateFailed
	case StateCheckpointed: return to == StateActive || to == StateSleeping || to == StateStopped || to == StateFailed
	case StateSleeping: return to == StateActive || to == StateStopped || to == StateFailed
	case StateStopped, StateFailed: return false
	default: return false
	}
}
