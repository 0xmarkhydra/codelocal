package runtimeprovider

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrManagedBackendMissing = errors.New("managed runtime backend missing")
	ErrManagedBackendRef     = errors.New("managed runtime backend returned empty reference")
	ErrManagedProviderHandle = errors.New("runtime handle does not belong to provider")
)

// ManagedBackend is the narrow bridge from Agent OS lifecycle semantics to a
// concrete local worker or cloud sandbox implementation. Durable task/agent
// identity remains outside the backend; the backend owns only the disposable
// runtime resource referenced by Handle.ID.
type ManagedBackend interface {
	Probe(context.Context) (available bool, reason string, err error)
	Provision(context.Context, Request) (resourceRef string, err error)
	Checkpoint(context.Context, string) (checkpointRef string, err error)
	Sleep(context.Context, string) error
	Resume(context.Context, string) error
	Stop(context.Context, string) error
}

type BackendFuncs struct {
	ProbeFunc      func(context.Context) (bool, string, error)
	ProvisionFunc  func(context.Context, Request) (string, error)
	CheckpointFunc func(context.Context, string) (string, error)
	SleepFunc      func(context.Context, string) error
	ResumeFunc     func(context.Context, string) error
	StopFunc       func(context.Context, string) error
}

func (b BackendFuncs) Probe(ctx context.Context) (bool, string, error) {
	if b.ProbeFunc == nil {
		return true, "", nil
	}
	return b.ProbeFunc(ctx)
}
func (b BackendFuncs) Provision(ctx context.Context, req Request) (string, error) {
	if b.ProvisionFunc == nil {
		return "", ErrManagedBackendMissing
	}
	return b.ProvisionFunc(ctx, req)
}
func (b BackendFuncs) Checkpoint(ctx context.Context, ref string) (string, error) {
	if b.CheckpointFunc == nil {
		return "", ErrManagedBackendMissing
	}
	return b.CheckpointFunc(ctx, ref)
}
func (b BackendFuncs) Sleep(ctx context.Context, ref string) error {
	if b.SleepFunc == nil {
		return ErrManagedBackendMissing
	}
	return b.SleepFunc(ctx, ref)
}
func (b BackendFuncs) Resume(ctx context.Context, ref string) error {
	if b.ResumeFunc == nil {
		return ErrManagedBackendMissing
	}
	return b.ResumeFunc(ctx, ref)
}
func (b BackendFuncs) Stop(ctx context.Context, ref string) error {
	if b.StopFunc == nil {
		return ErrManagedBackendMissing
	}
	return b.StopFunc(ctx, ref)
}

type ManagedProvider struct {
	id      string
	caps    Capabilities
	backend ManagedBackend
}

func NewManagedProvider(id string, caps Capabilities, backend ManagedBackend) (*ManagedProvider, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || backend == nil {
		return nil, ErrInvalidRuntimeProvider
	}
	return &ManagedProvider{id: id, caps: caps, backend: backend}, nil
}

func NewLocalProvider(caps Capabilities, backend ManagedBackend) (*ManagedProvider, error) {
	return NewManagedProvider("local", caps, backend)
}

func NewCloudProvider(caps Capabilities, backend ManagedBackend) (*ManagedProvider, error) {
	return NewManagedProvider("cloud", caps, backend)
}

func (p *ManagedProvider) ID() string {
	if p == nil {
		return ""
	}
	return p.id
}

func (p *ManagedProvider) Probe(ctx context.Context) Probe {
	probe := Probe{ProviderID: p.ID(), Capabilities: p.caps, CheckedAt: time.Now().UTC()}
	if p == nil || p.backend == nil {
		probe.Reason = ErrManagedBackendMissing.Error()
		return probe
	}
	available, reason, err := p.backend.Probe(ctx)
	probe.Available = available && err == nil
	probe.Reason = strings.Join(strings.Fields(reason), " ")
	if err != nil && probe.Reason == "" {
		probe.Reason = "backend_probe_failed"
	}
	return probe
}

func (p *ManagedProvider) Provision(ctx context.Context, req Request) (Handle, error) {
	if p == nil || p.backend == nil || !validManagedRequest(req) || !supports(p.caps, req.Required) {
		return Handle{}, ErrInvalidRuntimeProvider
	}
	probe := p.Probe(ctx)
	if !probe.Available {
		return Handle{}, ErrRuntimeProviderMissing
	}
	ref, err := p.backend.Provision(ctx, req)
	if err != nil {
		return Handle{}, err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Handle{}, ErrManagedBackendRef
	}
	now := time.Now().UTC()
	return Handle{ID: ref, ProviderID: p.id, WorkspaceKey: strings.TrimSpace(req.WorkspaceKey), TaskID: strings.TrimSpace(req.TaskID), AgentID: strings.TrimSpace(req.AgentID), ActivationID: strings.TrimSpace(req.ActivationID), State: StateActive, Capabilities: p.caps, Revision: 1, UpdatedAt: now}, nil
}

func (p *ManagedProvider) Checkpoint(ctx context.Context, handle Handle) (Handle, error) {
	preview, err := p.preview(handle, StateCheckpointed)
	if err != nil {
		return Handle{}, err
	}
	if !p.caps.DurableCheckpoint {
		return Handle{}, ErrRuntimeStateTransition
	}
	checkpoint, err := p.backend.Checkpoint(ctx, handle.ID)
	if err != nil {
		return Handle{}, err
	}
	checkpoint = strings.TrimSpace(checkpoint)
	if checkpoint == "" {
		return Handle{}, ErrManagedBackendRef
	}
	preview.CheckpointRef = checkpoint
	return preview, nil
}

func (p *ManagedProvider) Sleep(ctx context.Context, handle Handle) (Handle, error) {
	if !p.caps.DurableCheckpoint || strings.TrimSpace(handle.CheckpointRef) == "" {
		return Handle{}, ErrRuntimeStateTransition
	}
	preview, err := p.preview(handle, StateSleeping)
	if err != nil {
		return Handle{}, err
	}
	if err := p.backend.Sleep(ctx, handle.ID); err != nil {
		return Handle{}, err
	}
	return preview, nil
}

func (p *ManagedProvider) Resume(ctx context.Context, handle Handle) (Handle, error) {
	preview, err := p.preview(handle, StateActive)
	if err != nil {
		return Handle{}, err
	}
	if err := p.backend.Resume(ctx, handle.ID); err != nil {
		return Handle{}, err
	}
	return preview, nil
}

func (p *ManagedProvider) Stop(ctx context.Context, handle Handle) (Handle, error) {
	preview, err := p.preview(handle, StateStopped)
	if err != nil {
		return Handle{}, err
	}
	if err := p.backend.Stop(ctx, handle.ID); err != nil {
		return Handle{}, err
	}
	return preview, nil
}

func (p *ManagedProvider) preview(handle Handle, next State) (Handle, error) {
	if p == nil || p.backend == nil || handle.ProviderID != p.id || strings.TrimSpace(handle.ID) == "" || handle.Revision == 0 {
		return Handle{}, ErrManagedProviderHandle
	}
	return Transition(handle, handle.Revision, next, time.Now().UTC())
}

func validManagedRequest(req Request) bool {
	return strings.TrimSpace(req.WorkspaceKey) != "" && strings.TrimSpace(req.TaskID) != "" && strings.TrimSpace(req.AgentID) != "" && strings.TrimSpace(req.ActivationID) != ""
}
