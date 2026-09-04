package runtimeprovider

import (
	"context"
	"testing"
)

func TestManagedProviderLifecycle(t *testing.T) {
	backend := BackendFuncs{
		ProvisionFunc:  func(context.Context, Request) (string, error) { return "runtime-1", nil },
		CheckpointFunc: func(context.Context, string) (string, error) { return "checkpoint-1", nil },
		SleepFunc:      func(context.Context, string) error { return nil },
		ResumeFunc:     func(context.Context, string) error { return nil },
		StopFunc:       func(context.Context, string) error { return nil },
	}
	provider, err := NewLocalProvider(Capabilities{FileSystem: true, Shell: true, DurableCheckpoint: true}, backend)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := provider.Provision(context.Background(), Request{WorkspaceKey: "ws", TaskID: "task", AgentID: "agent", ActivationID: "activation", Required: Capabilities{FileSystem: true}})
	if err != nil {
		t.Fatal(err)
	}
	if handle.State != StateActive || handle.Revision != 1 {
		t.Fatalf("unexpected provisioned handle: %+v", handle)
	}
	handle, err = provider.Checkpoint(context.Background(), handle)
	if err != nil {
		t.Fatal(err)
	}
	if handle.State != StateCheckpointed || handle.CheckpointRef != "checkpoint-1" {
		t.Fatalf("unexpected checkpoint: %+v", handle)
	}
	handle, err = provider.Sleep(context.Background(), handle)
	if err != nil {
		t.Fatal(err)
	}
	if handle.State != StateSleeping {
		t.Fatalf("expected sleeping, got %s", handle.State)
	}
	handle, err = provider.Resume(context.Background(), handle)
	if err != nil {
		t.Fatal(err)
	}
	if handle.State != StateActive {
		t.Fatalf("expected active, got %s", handle.State)
	}
	handle, err = provider.Stop(context.Background(), handle)
	if err != nil {
		t.Fatal(err)
	}
	if handle.State != StateStopped {
		t.Fatalf("expected stopped, got %s", handle.State)
	}
}

func TestManagedProviderFailsClosed(t *testing.T) {
	provider, err := NewCloudProvider(Capabilities{FileSystem: true}, BackendFuncs{ProvisionFunc: func(context.Context, Request) (string, error) { return "runtime-1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Provision(context.Background(), Request{WorkspaceKey: "ws", TaskID: "task", AgentID: "agent", ActivationID: "activation", Required: Capabilities{Network: true}})
	if err == nil {
		t.Fatal("expected unsupported capability to fail")
	}
	handle := Handle{ID: "runtime-1", ProviderID: "cloud", WorkspaceKey: "ws", TaskID: "task", AgentID: "agent", ActivationID: "activation", State: StateActive, Revision: 1, Capabilities: Capabilities{FileSystem: true}}
	if _, err := provider.Sleep(context.Background(), handle); err == nil {
		t.Fatal("sleep without durable checkpoint must fail")
	}
}
