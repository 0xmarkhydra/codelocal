package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RuntimeProviderKind identifies an execution backend without leaking provider
// SDK objects into MCP or product-level workspace APIs.
type RuntimeProviderKind string

const (
	RuntimeProviderAuto  RuntimeProviderKind = "auto"
	RuntimeProviderLocal RuntimeProviderKind = "local"
	RuntimeProviderCloud RuntimeProviderKind = "cloud"
)

// ErrRuntimeProviderUnavailable is the only error class that allows Auto mode
// to try the next provider. Hard errors (authorization, invalid workspace,
// conflicts, etc.) must not be hidden by silently switching compute backends.
var ErrRuntimeProviderUnavailable = errors.New("runtime provider unavailable")

type RuntimeAcquireRequest struct {
	UserID       string
	WorkspaceKey string
	Mode         RuntimeProviderKind
}

type RuntimeCallRequest struct {
	UserID       string
	WorkspaceKey string
	SessionID    string
	Tool         string
	Args         map[string]any
	SideEffect   bool
	RequestID    string
}

// RuntimeProvider is the control-plane boundary for execution. The existing
// local runtime is the first implementation; OpenSandbox and future providers
// plug in here rather than into the runtime-node package.
type RuntimeProvider interface {
	Kind() RuntimeProviderKind
	Acquire(context.Context, RuntimeAcquireRequest) (*WorkspaceView, error)
	Call(context.Context, RuntimeCallRequest) (RoutedResult, error)
}

type RuntimeBinding struct {
	Kind      RuntimeProviderKind
	Workspace *WorkspaceView
	provider  RuntimeProvider
}

func (b *RuntimeBinding) Call(ctx context.Context, request RuntimeCallRequest) (RoutedResult, error) {
	if b == nil || b.provider == nil {
		return RoutedResult{}, errors.New("runtime binding unavailable")
	}
	if strings.TrimSpace(request.WorkspaceKey) == "" && b.Workspace != nil {
		request.WorkspaceKey = b.Workspace.Key
	}
	return b.provider.Call(ctx, request)
}

// RuntimeRouter selects a provider but deliberately does not own workspace
// persistence. Workspace identity/state remains durable outside compute.
type RuntimeRouter struct {
	providers []RuntimeProvider
	byKind    map[RuntimeProviderKind]RuntimeProvider
}

func NewRuntimeRouter(providers ...RuntimeProvider) *RuntimeRouter {
	router := &RuntimeRouter{byKind: map[RuntimeProviderKind]RuntimeProvider{}}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		kind := provider.Kind()
		if kind == "" || kind == RuntimeProviderAuto {
			continue
		}
		if _, exists := router.byKind[kind]; exists {
			continue
		}
		router.providers = append(router.providers, provider)
		router.byKind[kind] = provider
	}
	return router
}

func (r *RuntimeRouter) Acquire(ctx context.Context, request RuntimeAcquireRequest) (*RuntimeBinding, error) {
	if r == nil || len(r.providers) == 0 {
		return nil, errors.New("no runtime providers configured")
	}
	if strings.TrimSpace(request.UserID) == "" {
		return nil, errors.New("runtime userId required")
	}
	if strings.TrimSpace(request.WorkspaceKey) == "" {
		return nil, errors.New("runtime workspaceKey required")
	}
	mode := request.Mode
	if mode == "" {
		mode = RuntimeProviderAuto
	}
	if mode != RuntimeProviderAuto {
		provider := r.byKind[mode]
		if provider == nil {
			return nil, fmt.Errorf("runtime provider %q is not configured", mode)
		}
		workspace, err := provider.Acquire(ctx, request)
		if err != nil {
			return nil, err
		}
		return &RuntimeBinding{Kind: provider.Kind(), Workspace: workspace, provider: provider}, nil
	}

	var unavailable error
	for _, provider := range r.providers {
		providerRequest := request
		providerRequest.Mode = provider.Kind()
		workspace, err := provider.Acquire(ctx, providerRequest)
		if err == nil {
			return &RuntimeBinding{Kind: provider.Kind(), Workspace: workspace, provider: provider}, nil
		}
		if !errors.Is(err, ErrRuntimeProviderUnavailable) {
			return nil, err
		}
		unavailable = err
	}
	if unavailable != nil {
		return nil, unavailable
	}
	return nil, ErrRuntimeProviderUnavailable
}

type LocalRuntimeProvider struct {
	Workspaces *WorkspaceService
	Hub        *Hub
}

func NewLocalRuntimeProvider(workspaces *WorkspaceService, hub *Hub) *LocalRuntimeProvider {
	return &LocalRuntimeProvider{Workspaces: workspaces, Hub: hub}
}

func (p *LocalRuntimeProvider) Kind() RuntimeProviderKind { return RuntimeProviderLocal }

func (p *LocalRuntimeProvider) Acquire(ctx context.Context, request RuntimeAcquireRequest) (*WorkspaceView, error) {
	if p == nil || p.Workspaces == nil {
		return nil, errors.New("local runtime workspace service unavailable")
	}
	return p.Workspaces.Activate(ctx, request.UserID, request.WorkspaceKey)
}

func (p *LocalRuntimeProvider) Call(ctx context.Context, request RuntimeCallRequest) (RoutedResult, error) {
	if p == nil || p.Hub == nil {
		return RoutedResult{}, errors.New("local runtime gateway unavailable")
	}
	return p.Hub.Call(
		ctx,
		request.UserID,
		request.WorkspaceKey,
		request.SessionID,
		request.Tool,
		request.Args,
		request.SideEffect,
		request.RequestID,
	)
}
