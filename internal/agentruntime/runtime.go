package agentruntime

import (
	"context"
	"strings"
)

type Runtime struct {
	Registry *Registry
}

func NewRuntime(registry *Registry) *Runtime {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Runtime{Registry: registry}
}

func (r *Runtime) Start(ctx context.Context, engineID string, request Request) (Session, error) {
	engineID = strings.ToLower(strings.TrimSpace(engineID))
	if engineID == "" || engineID == "auto" {
		return nil, ErrAutoRoutingDisabled
	}
	request, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	adapter, ok := r.Registry.Get(engineID)
	if !ok {
		return nil, ErrEngineNotFound
	}
	capabilities := adapter.Capabilities()
	if request.Mode == ModeMutate && !mutationIsolationAllowed(capabilities) {
		return nil, ErrUnsafeMutation
	}
	probe := adapter.Probe(ctx)
	if !probe.Installed || !probe.Compatible {
		return nil, ErrEngineNotFound
	}
	return adapter.Start(ctx, request)
}
