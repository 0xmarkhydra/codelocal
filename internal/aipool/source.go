package aipool

import (
	"context"
	"net/http"
	"time"
)

// Source is one upstream capable of serving one or more canonical models.
// Provider-specific model IDs never cross the Pool client boundary.
type Source interface {
	ID() string
	Name() string
	Kind() string
	Priority() int
	Models(context.Context) ([]UpstreamModel, error)
	Do(context.Context, *http.Request, string) (*http.Response, error)
}

type UpstreamModel struct {
	Canonical string
	Upstream  string
}

type SourceState string

const (
	SourceHealthy      SourceState = "healthy"
	SourceDegraded     SourceState = "degraded"
	SourceExhausted    SourceState = "exhausted"
	SourceUnauthorized SourceState = "unauthorized"
	SourceCooldown     SourceState = "cooldown"
	SourceDisabled     SourceState = "disabled"
)

type ModelSourceStatus struct {
	SourceID  string      `json:"sourceId"`
	Source    string      `json:"source"`
	Kind      string      `json:"kind"`
	State     SourceState `json:"state"`
	RetryAt   time.Time   `json:"retryAt,omitempty"`
	LastError string      `json:"lastError,omitempty"`
}

type CanonicalModel struct {
	ID               string              `json:"id"`
	Active           bool                `json:"active"`
	State            string              `json:"state"`
	AvailableSources int                 `json:"availableSources"`
	TotalSources     int                 `json:"totalSources"`
	Sources          []ModelSourceStatus `json:"sources,omitempty"`
}

type SourceSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"baseUrl"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
	ManagedBy string `json:"managedBy"`
}
