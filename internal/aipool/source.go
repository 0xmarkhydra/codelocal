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
	Status    *ModelSourceStatus
}

type SourceState string

const (
	SourceHealthy      SourceState = "healthy"
	SourceDegraded     SourceState = "degraded"
	SourceExhausted    SourceState = "exhausted"
	SourceUnauthorized SourceState = "unauthorized"
	SourceCooldown     SourceState = "cooldown"
	SourceUnavailable  SourceState = "unavailable"
	SourceDisabled     SourceState = "disabled"
)

type ModelSourceStatus struct {
	SourceID              string      `json:"sourceId"`
	Source                string      `json:"source"`
	Kind                  string      `json:"kind"`
	Provider              string      `json:"provider,omitempty"`
	Upstream              string      `json:"upstream,omitempty"`
	State                 SourceState `json:"state"`
	AvailableRoutes       int         `json:"availableRoutes,omitempty"`
	TotalRoutes           int         `json:"totalRoutes,omitempty"`
	QuotaRemainingPercent *float64    `json:"quotaRemainingPercent,omitempty"`
	QuotaResetAt          time.Time   `json:"quotaResetAt,omitempty"`
	RetryAt               time.Time   `json:"retryAt,omitempty"`
	LastCheckedAt         time.Time   `json:"lastCheckedAt,omitempty"`
	LastError             string      `json:"lastError,omitempty"`
}

type CanonicalModel struct {
	ID               string              `json:"id"`
	Active           bool                `json:"active"`
	State            string              `json:"state"`
	AvailableSources int                 `json:"availableSources"`
	TotalSources     int                 `json:"totalSources"`
	AvailableRoutes  int                 `json:"availableRoutes"`
	TotalRoutes      int                 `json:"totalRoutes"`
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

type ModelTestResult struct {
	Model      string `json:"model"`
	OK         bool   `json:"ok"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Source     string `json:"source,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Upstream   string `json:"upstream,omitempty"`
	LatencyMS  int64  `json:"latencyMs,omitempty"`
	Attempts   int    `json:"attempts"`
	Error      string `json:"error,omitempty"`
}
