package gateway

import (
	"context"
	"errors"
	"testing"
)

type fakeRuntimeProvider struct {
	kind         RuntimeProviderKind
	workspace    *WorkspaceView
	acquireErr   error
	callResult   RoutedResult
	callErr      error
	acquireCount int
	callCount    int
}

func (p *fakeRuntimeProvider) Kind() RuntimeProviderKind { return p.kind }

func (p *fakeRuntimeProvider) Acquire(context.Context, RuntimeAcquireRequest) (*WorkspaceView, error) {
	p.acquireCount++
	return p.workspace, p.acquireErr
}

func (p *fakeRuntimeProvider) Call(context.Context, RuntimeCallRequest) (RoutedResult, error) {
	p.callCount++
	return p.callResult, p.callErr
}

func TestRuntimeRouterAutoUsesFirstEligibleProvider(t *testing.T) {
	local := &fakeRuntimeProvider{kind: RuntimeProviderLocal, workspace: &WorkspaceView{Key: "local-key"}}
	cloud := &fakeRuntimeProvider{kind: RuntimeProviderCloud, workspace: &WorkspaceView{Key: "cloud-key"}}
	router := NewRuntimeRouter(local, cloud)

	binding, err := router.Acquire(context.Background(), RuntimeAcquireRequest{UserID: "user", WorkspaceKey: "workspace", Mode: RuntimeProviderAuto})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if binding.Kind != RuntimeProviderLocal || binding.Workspace.Key != "local-key" {
		t.Fatalf("Acquire() binding = %#v, want local provider", binding)
	}
	if local.acquireCount != 1 || cloud.acquireCount != 0 {
		t.Fatalf("acquire counts local=%d cloud=%d, want 1/0", local.acquireCount, cloud.acquireCount)
	}
}

func TestRuntimeRouterAutoFallsBackOnlyWhenProviderUnavailable(t *testing.T) {
	local := &fakeRuntimeProvider{kind: RuntimeProviderLocal, acquireErr: errors.Join(ErrRuntimeProviderUnavailable, errors.New("local offline"))}
	cloud := &fakeRuntimeProvider{kind: RuntimeProviderCloud, workspace: &WorkspaceView{Key: "cloud-key"}}
	router := NewRuntimeRouter(local, cloud)

	binding, err := router.Acquire(context.Background(), RuntimeAcquireRequest{UserID: "user", WorkspaceKey: "workspace"})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if binding.Kind != RuntimeProviderCloud {
		t.Fatalf("Acquire() provider = %q, want cloud", binding.Kind)
	}
	if local.acquireCount != 1 || cloud.acquireCount != 1 {
		t.Fatalf("acquire counts local=%d cloud=%d, want 1/1", local.acquireCount, cloud.acquireCount)
	}
}

func TestRuntimeRouterAutoDoesNotHideHardProviderError(t *testing.T) {
	hardErr := errors.New("workspace unauthorized")
	local := &fakeRuntimeProvider{kind: RuntimeProviderLocal, acquireErr: hardErr}
	cloud := &fakeRuntimeProvider{kind: RuntimeProviderCloud, workspace: &WorkspaceView{Key: "cloud-key"}}
	router := NewRuntimeRouter(local, cloud)

	_, err := router.Acquire(context.Background(), RuntimeAcquireRequest{UserID: "user", WorkspaceKey: "workspace"})
	if !errors.Is(err, hardErr) {
		t.Fatalf("Acquire() error = %v, want %v", err, hardErr)
	}
	if cloud.acquireCount != 0 {
		t.Fatalf("cloud acquire count = %d, want 0", cloud.acquireCount)
	}
}

func TestRuntimeRouterExplicitProviderWins(t *testing.T) {
	local := &fakeRuntimeProvider{kind: RuntimeProviderLocal, workspace: &WorkspaceView{Key: "local-key"}}
	cloud := &fakeRuntimeProvider{kind: RuntimeProviderCloud, workspace: &WorkspaceView{Key: "cloud-key"}}
	router := NewRuntimeRouter(local, cloud)

	binding, err := router.Acquire(context.Background(), RuntimeAcquireRequest{UserID: "user", WorkspaceKey: "workspace", Mode: RuntimeProviderCloud})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if binding.Kind != RuntimeProviderCloud || binding.Workspace.Key != "cloud-key" {
		t.Fatalf("Acquire() binding = %#v, want cloud provider", binding)
	}
	if local.acquireCount != 0 || cloud.acquireCount != 1 {
		t.Fatalf("acquire counts local=%d cloud=%d, want 0/1", local.acquireCount, cloud.acquireCount)
	}
}

func TestRuntimeBindingDelegatesCallAndDefaultsWorkspaceKey(t *testing.T) {
	provider := &fakeRuntimeProvider{kind: RuntimeProviderLocal, callResult: RoutedResult{OK: true, Result: "ok"}}
	binding := &RuntimeBinding{Kind: RuntimeProviderLocal, Workspace: &WorkspaceView{Key: "workspace-key"}, provider: provider}

	result, err := binding.Call(context.Background(), RuntimeCallRequest{UserID: "user", Tool: "read_file"})
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if !result.OK || provider.callCount != 1 {
		t.Fatalf("Call() result=%#v callCount=%d", result, provider.callCount)
	}
}

func TestRuntimeRouterRejectsUnknownExplicitProvider(t *testing.T) {
	router := NewRuntimeRouter(&fakeRuntimeProvider{kind: RuntimeProviderLocal})
	_, err := router.Acquire(context.Background(), RuntimeAcquireRequest{UserID: "user", WorkspaceKey: "workspace", Mode: RuntimeProviderCloud})
	if err == nil {
		t.Fatal("Acquire() error = nil, want unsupported provider error")
	}
}
