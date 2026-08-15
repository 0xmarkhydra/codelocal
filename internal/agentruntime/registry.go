package agentruntime

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: map[string]Adapter{}}
}

func (r *Registry) Register(adapter Adapter) error {
	if r == nil || adapter == nil || strings.TrimSpace(adapter.ID()) == "" {
		return ErrInvalidAgentRequest
	}
	id := strings.ToLower(strings.TrimSpace(adapter.ID()))
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = map[string]Adapter{}
	}
	if _, exists := r.adapters[id]; exists {
		return ErrEngineAlreadyExists
	}
	r.adapters[id] = adapter
	return nil
}

func (r *Registry) Get(id string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	id = strings.ToLower(strings.TrimSpace(id))
	r.mu.RLock()
	adapter, ok := r.adapters[id]
	r.mu.RUnlock()
	return adapter, ok
}

func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Strings(ids)
	return ids
}

func (r *Registry) ProbeAll(ctx context.Context) []ProbeResult {
	ids := r.IDs()
	results := make([]ProbeResult, 0, len(ids))
	for _, id := range ids {
		adapter, ok := r.Get(id)
		if !ok {
			continue
		}
		probe := adapter.Probe(ctx)
		probe.EngineID = id
		if probe.DisplayName == "" {
			probe.DisplayName = adapter.DisplayName()
		}
		if probe.CheckedAt == 0 {
			probe.CheckedAt = nowMillis()
		}
		if probe.Capabilities.Transport == "" {
			probe.Capabilities = adapter.Capabilities()
		}
		results = append(results, probe)
	}
	return results
}
