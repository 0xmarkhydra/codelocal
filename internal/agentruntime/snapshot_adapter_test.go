package agentruntime

import (
	"context"
	"errors"
	"testing"
)

func TestSnapshotAdapterIsProbeOnly(t *testing.T) {
	adapter := NewSnapshotAdapter("Host-Shadow", "", Capabilities{
		NonInteractive:   true,
		StructuredOutput: true,
		FileEditing:      true,
		ShellExecution:   true,
	})

	if got := adapter.ID(); got != "host-shadow" {
		t.Fatalf("ID() = %q, want host-shadow", got)
	}
	probe := adapter.Probe(context.Background())
	if !probe.Installed || !probe.Compatible {
		t.Fatalf("probe should advertise an available routing candidate: %+v", probe)
	}
	if probe.Capabilities.Transport != TransportNativeStructured {
		t.Fatalf("transport = %q, want %q", probe.Capabilities.Transport, TransportNativeStructured)
	}
	if probe.Capabilities.Isolation != IsolationMediated {
		t.Fatalf("isolation = %q, want %q", probe.Capabilities.Isolation, IsolationMediated)
	}

	_, err := adapter.Start(context.Background(), Request{})
	if !errors.Is(err, ErrSnapshotExecutionDisabled) {
		t.Fatalf("Start() error = %v, want ErrSnapshotExecutionDisabled", err)
	}
}

func TestSnapshotAdapterCanParticipateInRegistryButNotRuntimeExecution(t *testing.T) {
	registry := NewRegistry()
	adapter := NewSnapshotAdapter("codelocal-host-shadow", "CodeLocal Host", Capabilities{
		NonInteractive: true,
		FileEditing:    true,
		Isolation:      IsolationMediated,
	})
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := registry.IDs(); len(got) != 1 || got[0] != adapter.ID() {
		t.Fatalf("registry IDs = %v", got)
	}

	runtime := NewRuntime(registry)
	_, err := runtime.Start(context.Background(), adapter.ID(), Request{
		TaskID:       "task-1",
		WorkspaceKey: "workspace-1",
		Objective:    "review current state",
		Mode:         ModeReview,
	})
	if !errors.Is(err, ErrSnapshotExecutionDisabled) {
		t.Fatalf("Runtime.Start() error = %v, want snapshot execution disabled", err)
	}
}
