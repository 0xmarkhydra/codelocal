package taskexecution

import (
	"errors"
	"strings"
)

type LifecycleState string

const (
	LifecycleCold         LifecycleState = "cold"
	LifecycleProvisioning LifecycleState = "provisioning"
	LifecycleReady        LifecycleState = "ready"
	LifecycleActive       LifecycleState = "active"
	LifecycleIdle         LifecycleState = "idle"
	LifecycleCheckpointed LifecycleState = "checkpointed"
	LifecycleSleeping     LifecycleState = "sleeping"
	LifecycleTerminated   LifecycleState = "terminated"
)

var (
	ErrInvalidLifecycleTransition = errors.New("invalid runtime lifecycle transition")
	ErrLifecycleSleepUnsafe       = errors.New("sleep requires a resume checkpoint")
)

// RuntimeLifecycle tracks the host lifecycle underneath a task. Sleep never
// destroys resumable state: entering sleeping requires a checkpoint digest
// produced by ExportTaskResume, so a woken runtime replays from known truth.
type RuntimeLifecycle struct {
	State            LifecycleState "state"
	CheckpointDigest string         "checkpointDigest,omitempty"
}

func (l RuntimeLifecycle) transition(to LifecycleState) (RuntimeLifecycle, error) {
	allowed := map[LifecycleState][]LifecycleState{
		LifecycleCold:         {LifecycleProvisioning, LifecycleTerminated},
		LifecycleProvisioning: {LifecycleReady, LifecycleTerminated},
		LifecycleReady:        {LifecycleActive, LifecycleTerminated},
		LifecycleActive:       {LifecycleIdle, LifecycleCheckpointed, LifecycleTerminated},
		LifecycleIdle:         {LifecycleActive, LifecycleCheckpointed, LifecycleTerminated},
		LifecycleCheckpointed: {LifecycleIdle, LifecycleSleeping, LifecycleTerminated},
		LifecycleSleeping:     {LifecycleReady, LifecycleTerminated},
		LifecycleTerminated:   {},
	}
	for _, next := range allowed[l.State] {
		if next == to {
			l.State = to
			return l, nil
		}
	}
	return RuntimeLifecycle{}, ErrInvalidLifecycleTransition
}

func (l RuntimeLifecycle) Provision() (RuntimeLifecycle, error) {
	return l.transition(LifecycleProvisioning)
}

func (l RuntimeLifecycle) MarkReady() (RuntimeLifecycle, error) {
	next, err := l.transition(LifecycleReady)
	if err != nil {
		return RuntimeLifecycle{}, err
	}
	next.CheckpointDigest = ""
	return next, nil
}

func (l RuntimeLifecycle) Activate() (RuntimeLifecycle, error) {
	return l.transition(LifecycleActive)
}

func (l RuntimeLifecycle) MarkIdle() (RuntimeLifecycle, error) {
	return l.transition(LifecycleIdle)
}

func (l RuntimeLifecycle) Checkpoint(digest string) (RuntimeLifecycle, error) {
	if strings.TrimSpace(digest) == "" {
		return RuntimeLifecycle{}, ErrInvalidLifecycleTransition
	}
	next, err := l.transition(LifecycleCheckpointed)
	if err != nil {
		return RuntimeLifecycle{}, err
	}
	next.CheckpointDigest = strings.TrimSpace(digest)
	return next, nil
}

func (l RuntimeLifecycle) Sleep() (RuntimeLifecycle, error) {
	if strings.TrimSpace(l.CheckpointDigest) == "" {
		return RuntimeLifecycle{}, ErrLifecycleSleepUnsafe
	}
	next, err := l.transition(LifecycleSleeping)
	if err != nil {
		return RuntimeLifecycle{}, err
	}
	return next, nil
}

func (l RuntimeLifecycle) Wake() (RuntimeLifecycle, error) {
	next, err := l.transition(LifecycleReady)
	if err != nil {
		return RuntimeLifecycle{}, err
	}
	next.CheckpointDigest = ""
	return next, nil
}

func (l RuntimeLifecycle) Terminate() (RuntimeLifecycle, error) {
	if l.State == LifecycleTerminated {
		return RuntimeLifecycle{}, ErrInvalidLifecycleTransition
	}
	return l.transition(LifecycleTerminated)
}
