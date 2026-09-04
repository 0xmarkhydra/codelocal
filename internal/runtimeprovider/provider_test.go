package runtimeprovider

import (
	"errors"
	"testing"
	"time"
)

func TestChooseLeastPrivilegeProvider(t *testing.T) {
	probes := []Probe{
		{ProviderID: "wide", Available: true, Capabilities: Capabilities{FileSystem: true, Shell: true, Network: true, Browser: true, Computer: true, Mobile: true, DurableCheckpoint: true}},
		{ProviderID: "local", Available: true, Capabilities: Capabilities{FileSystem: true, Shell: true}},
	}
	chosen, err := Choose(probes, Capabilities{FileSystem: true, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	if chosen.ProviderID != "local" {
		t.Fatalf("expected least privilege provider, got %+v", chosen)
	}
}

func TestRuntimeLifecycleSupportsSleepResume(t *testing.T) {
	h := Handle{ID: "h", ProviderID: "cloud", State: StateCold, Revision: 1}
	var err error
	for _, next := range []State{StateProvisioning, StateActive, StateIdle, StateCheckpointed, StateSleeping, StateActive, StateStopped} {
		h, err = Transition(h, h.Revision, next, time.Time{})
		if err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if _, err := Transition(h, h.Revision, StateActive, time.Time{}); !errors.Is(err, ErrRuntimeStateTransition) {
		t.Fatalf("stopped runtime resurrected: %v", err)
	}
}

func TestRuntimeLifecycleRejectsStaleRevision(t *testing.T) {
	h := Handle{ID: "h", ProviderID: "local", State: StateCold, Revision: 2}
	if _, err := Transition(h, 1, StateProvisioning, time.Time{}); !errors.Is(err, ErrRuntimeStateTransition) {
		t.Fatalf("stale runtime revision accepted: %v", err)
	}
}
