package aipool

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type SourceRegistry struct {
	static []Source
	store  *SourceStore
	ttl    time.Duration

	mu        sync.Mutex
	cachedAt  time.Time
	cached    []Source
	summaries []SourceSummary
}

func NewSourceRegistry(store *SourceStore, static ...Source) *SourceRegistry {
	filtered := make([]Source, 0, len(static))
	for _, source := range static {
		if source != nil {
			filtered = append(filtered, source)
		}
	}
	return &SourceRegistry{static: filtered, store: store, ttl: 5 * time.Second}
}

func (r *SourceRegistry) invalidate() {
	r.mu.Lock()
	r.cachedAt = time.Time{}
	r.cached = nil
	r.summaries = nil
	r.mu.Unlock()
}

func (r *SourceRegistry) Sources(ctx context.Context) ([]Source, error) {
	r.mu.Lock()
	if !r.cachedAt.IsZero() && time.Since(r.cachedAt) < r.ttl {
		out := append([]Source(nil), r.cached...)
		r.mu.Unlock()
		return out, nil
	}
	r.mu.Unlock()

	sources := append([]Source(nil), r.static...)
	var loadErr error
	if r.store != nil {
		records, err := r.store.List(ctx, false)
		if err != nil {
			loadErr = err
		} else {
			for _, record := range records {
				source, sourceErr := sourceFromStored(record)
				if sourceErr != nil {
					loadErr = sourceErr
					continue
				}
				sources = append(sources, source)
			}
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Priority() != sources[j].Priority() {
			return sources[i].Priority() < sources[j].Priority()
		}
		return strings.ToLower(sources[i].ID()) < strings.ToLower(sources[j].ID())
	})
	if len(sources) == 0 {
		if loadErr != nil {
			return nil, loadErr
		}
		return nil, errors.New("Pool has no enabled sources")
	}

	r.mu.Lock()
	r.cachedAt = time.Now()
	r.cached = append([]Source(nil), sources...)
	r.summaries = summariesForSources(sources)
	r.mu.Unlock()
	return sources, nil
}

func sourceFromStored(record StoredSource) (Source, error) {
	kind := strings.ToLower(strings.TrimSpace(record.Kind))
	switch kind {
	case "openai", "openai-compatible", "openrouter", "custom":
		return NewOpenAISource(OpenAISourceOptions{
			ID:       record.ID,
			Name:     record.Name,
			Kind:     kind,
			BaseURL:  record.BaseURL,
			APIKey:   record.APIKey,
			Priority: record.Priority,
		})
	default:
		return nil, errors.New("unsupported Pool source kind: " + kind)
	}
}

func summariesForSources(sources []Source) []SourceSummary {
	out := make([]SourceSummary, 0, len(sources))
	for _, source := range sources {
		baseURL := ""
		managedBy := "pool"
		if item, ok := source.(*OpenAISource); ok {
			baseURL = item.BaseURL()
			if item.Kind() == "9router" {
				managedBy = "environment"
			}
		}
		out = append(out, SourceSummary{
			ID: source.ID(), Name: source.Name(), Kind: source.Kind(), BaseURL: baseURL,
			Priority: source.Priority(), Enabled: true, ManagedBy: managedBy,
		})
	}
	return out
}

func (r *SourceRegistry) Summaries(ctx context.Context) ([]SourceSummary, error) {
	if _, err := r.Sources(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	out := append([]SourceSummary(nil), r.summaries...)
	r.mu.Unlock()
	if r.store != nil {
		records, err := r.store.List(ctx, true)
		if err == nil {
			byID := map[string]bool{}
			for _, item := range out {
				byID[item.ID] = true
			}
			for _, record := range records {
				if byID[record.ID] {
					continue
				}
				out = append(out, sourceSummaryFromStored(record))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out, nil
}

func (r *SourceRegistry) Create(ctx context.Context, source StoredSource) (SourceSummary, error) {
	if r.store == nil {
		return SourceSummary{}, errors.New("Pool source storage is not configured")
	}
	if strings.TrimSpace(source.ID) == "" {
		id, err := randomSourceID()
		if err != nil {
			return SourceSummary{}, err
		}
		source.ID = id
	}
	candidate, err := sourceFromStored(source)
	if err != nil {
		return SourceSummary{}, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if _, err := candidate.Models(probeCtx); err != nil {
		return SourceSummary{}, errors.New("source validation failed: " + err.Error())
	}
	created, err := r.store.Create(ctx, source)
	if err != nil {
		return SourceSummary{}, err
	}
	r.invalidate()
	return sourceSummaryFromStored(created), nil
}

func (r *SourceRegistry) SetEnabled(ctx context.Context, id string, enabled bool) error {
	if r.store == nil {
		return errors.New("Pool source storage is not configured")
	}
	if err := r.store.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	r.invalidate()
	return nil
}

func (r *SourceRegistry) Delete(ctx context.Context, id string) error {
	if r.store == nil {
		return errors.New("Pool source storage is not configured")
	}
	if err := r.store.Delete(ctx, id); err != nil {
		return err
	}
	r.invalidate()
	return nil
}
