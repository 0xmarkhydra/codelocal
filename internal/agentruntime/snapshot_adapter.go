package agentruntime

import (
	"context"
	"errors"
	"strings"
)

var ErrSnapshotExecutionDisabled = errors.New("snapshot adapter cannot execute agent sessions")

// SnapshotAdapter exposes a capability snapshot to routing and planning without
// owning an execution surface. It is intentionally probe-only so shadow/canary
// planning can never double-execute a user task.
type SnapshotAdapter struct {
	id          string
	displayName string
	caps        Capabilities
}

func NewSnapshotAdapter(id, displayName string, caps Capabilities) *SnapshotAdapter {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		id = "codelocal-host-shadow"
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "CodeLocal Host (shadow)"
	}
	if caps.Transport == "" {
		caps.Transport = TransportNativeStructured
	}
	if caps.Isolation == "" {
		caps.Isolation = IsolationMediated
	}
	return &SnapshotAdapter{id: id, displayName: displayName, caps: caps}
}

func (a *SnapshotAdapter) ID() string { return a.id }

func (a *SnapshotAdapter) DisplayName() string { return a.displayName }

func (a *SnapshotAdapter) Capabilities() Capabilities { return a.caps }

func (a *SnapshotAdapter) Probe(context.Context) ProbeResult {
	return ProbeResult{
		EngineID:     a.id,
		DisplayName:  a.displayName,
		Installed:    true,
		Auth:         AuthAuthenticated,
		Capabilities: a.caps,
		Compatible:   true,
		Reason:       "capability snapshot for shadow routing only",
		CheckedAt:    nowMillis(),
	}
}

func (a *SnapshotAdapter) Start(context.Context, Request) (Session, error) {
	return nil, ErrSnapshotExecutionDisabled
}
