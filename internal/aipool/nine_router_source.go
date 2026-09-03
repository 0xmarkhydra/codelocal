package aipool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type NineRouterSourceOptions struct {
	BaseURL       string
	APIKey        string
	AdminPassword string
	Priority      int
}

type NineRouterSource struct {
	inference     *OpenAISource
	managementURL string
	adminPassword string
	priority      int
	client        *http.Client
	now           func() time.Time
	statusTTL     time.Duration

	sessionMu      sync.Mutex
	sessionCookie  string
	sessionExpires time.Time

	snapshotMu sync.Mutex
	snapshot   nineRouterManagementSnapshot
}

type nineRouterConnection struct {
	ID         string `json:"id"`
	Provider   string `json:"provider"`
	AuthType   string `json:"authType"`
	TestStatus string `json:"testStatus"`
	IsActive   bool   `json:"isActive"`
	Priority   int    `json:"priority"`
}

type nineRouterAvailability struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Status       string `json:"status"`
	Until        string `json:"until"`
	ConnectionID string `json:"connectionId"`
}

type nineRouterUsage struct {
	Status int
	Data   map[string]any
	Err    error
}

type nineRouterManagementSnapshot struct {
	FetchedAt    time.Time
	Connections  []nineRouterConnection
	Availability []nineRouterAvailability
	Usage        map[string]nineRouterUsage
}

type nineRouterRouteAssessment struct {
	State      SourceState
	QuotaPct   *float64
	ResetAt    time.Time
	RetryAt    time.Time
	LastError  string
	QuotaKnown bool
}

type nineRouterQuotaAssessment struct {
	Known     bool
	Exhausted bool
	Percent   *float64
	ResetAt   time.Time
}

func NewNineRouterSource(opts NineRouterSourceOptions) (*NineRouterSource, error) {
	inference, err := NewOpenAISource(OpenAISourceOptions{
		ID:            "9router",
		Name:          "9Router",
		Kind:          "9router",
		BaseURL:       opts.BaseURL,
		APIKey:        opts.APIKey,
		Priority:      opts.Priority,
		StripPrefixes: true,
	})
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(inference.BaseURL())
	if err != nil {
		return nil, err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(path), "/v1") {
		path = strings.TrimSuffix(path, path[len(path)-3:])
	}
	parsed.Path = strings.TrimRight(path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	priority := opts.Priority
	if priority < 0 {
		priority = 0
	}
	return &NineRouterSource{
		inference:     inference,
		managementURL: strings.TrimRight(parsed.String(), "/"),
		adminPassword: strings.TrimSpace(opts.AdminPassword),
		priority:      priority,
		client: &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   24,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
		}},
		now:       time.Now,
		statusTTL: 15 * time.Second,
	}, nil
}

func (s *NineRouterSource) ID() string      { return "9router" }
func (s *NineRouterSource) Name() string    { return "9Router" }
func (s *NineRouterSource) Kind() string    { return "9router" }
func (s *NineRouterSource) Priority() int   { return s.priority }
func (s *NineRouterSource) BaseURL() string { return s.inference.BaseURL() }

func (s *NineRouterSource) Do(ctx context.Context, incoming *http.Request, upstreamModel string) (*http.Response, error) {
	return s.inference.Do(ctx, incoming, upstreamModel)
}

func (s *NineRouterSource) Models(ctx context.Context) ([]UpstreamModel, error) {
	models, err := s.inference.Models(ctx)
	if err != nil {
		return nil, err
	}
	if s.adminPassword == "" {
		now := s.now()
		for i := range models {
			models[i].Status = &ModelSourceStatus{
				SourceID: "9router", Source: "9Router", Kind: "9router",
				Provider: nineRouterProviderForModel(models[i].Upstream), Upstream: models[i].Upstream,
				State: SourceDegraded, AvailableRoutes: 1, TotalRoutes: 1,
				LastCheckedAt: now, LastError: "9Router management access is not configured",
			}
		}
		return models, nil
	}

	snapshot, snapshotErr := s.managementSnapshot(ctx)
	for i := range models {
		status := ModelSourceStatus{
			SourceID: "9router", Source: "9Router", Kind: "9router",
			Provider: nineRouterProviderForModel(models[i].Upstream), Upstream: models[i].Upstream,
		}
		if snapshotErr != nil {
			status.State = SourceDegraded
			status.AvailableRoutes = 1
			status.TotalRoutes = 1
			status.LastCheckedAt = s.now()
			status.LastError = "9Router management state is temporarily unavailable"
		} else {
			status = s.statusForModel(models[i], snapshot)
		}
		models[i].Status = &status
	}
	return models, nil
}

func (s *NineRouterSource) managementSnapshot(ctx context.Context) (nineRouterManagementSnapshot, error) {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	if !s.snapshot.FetchedAt.IsZero() && s.now().Sub(s.snapshot.FetchedAt) < s.statusTTL {
		return s.snapshot, nil
	}

	var providerPayload struct {
		Connections []nineRouterConnection `json:"connections"`
	}
	if err := s.managementJSON(ctx, "/api/providers", &providerPayload); err != nil {
		if !s.snapshot.FetchedAt.IsZero() {
			return s.snapshot, nil
		}
		return nineRouterManagementSnapshot{}, err
	}
	var availabilityPayload struct {
		Models []nineRouterAvailability `json:"models"`
	}
	if err := s.managementJSON(ctx, "/api/models/availability", &availabilityPayload); err != nil {
		if !s.snapshot.FetchedAt.IsZero() {
			return s.snapshot, nil
		}
		return nineRouterManagementSnapshot{}, err
	}

	usage := make(map[string]nineRouterUsage, len(providerPayload.Connections))
	var usageMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, connection := range providerPayload.Connections {
		if strings.TrimSpace(connection.ID) == "" || !connection.IsActive {
			continue
		}
		connection := connection
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			var data map[string]any
			status, err := s.managementJSONStatus(ctx, "/api/usage/"+url.PathEscape(connection.ID), &data)
			usageMu.Lock()
			usage[connection.ID] = nineRouterUsage{Status: status, Data: data, Err: err}
			usageMu.Unlock()
		}()
	}
	wg.Wait()

	snapshot := nineRouterManagementSnapshot{
		FetchedAt:    s.now(),
		Connections:  providerPayload.Connections,
		Availability: availabilityPayload.Models,
		Usage:        usage,
	}
	s.snapshot = snapshot
	return snapshot, nil
}

func (s *NineRouterSource) managementJSON(ctx context.Context, path string, target any) error {
	status, err := s.managementJSONStatus(ctx, path, target)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("9Router management API returned status %d", status)
	}
	return nil
}

func (s *NineRouterSource) managementJSONStatus(ctx context.Context, path string, target any) (int, error) {
	for attempt := 0; attempt < 2; attempt++ {
		cookie, err := s.dashboardSession(ctx, attempt > 0)
		if err != nil {
			return 0, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.managementURL+path, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Cookie", cookie)
		resp, err := s.client.Do(req)
		if err != nil {
			return 0, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return resp.StatusCode, readErr
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			s.clearDashboardSession()
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return resp.StatusCode, fmt.Errorf("9Router management API returned status %d", resp.StatusCode)
		}
		if target != nil && len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, target); err != nil {
				return resp.StatusCode, err
			}
		}
		return resp.StatusCode, nil
	}
	return http.StatusUnauthorized, errors.New("9Router dashboard session is unauthorized")
}

func (s *NineRouterSource) dashboardSession(ctx context.Context, force bool) (string, error) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if !force && s.sessionCookie != "" && s.now().Before(s.sessionExpires) {
		return s.sessionCookie, nil
	}
	payload, err := json.Marshal(map[string]string{"password": s.adminPassword})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.managementURL+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("9Router dashboard login returned status %d", resp.StatusCode)
	}
	cookie := ""
	for _, item := range resp.Cookies() {
		if item.Name == "auth_token" {
			cookie = item.Name + "=" + item.Value
			break
		}
		if cookie == "" && item.Name != "" {
			cookie = item.Name + "=" + item.Value
		}
	}
	if cookie == "" {
		return "", errors.New("9Router dashboard login did not return a session cookie")
	}
	s.sessionCookie = cookie
	s.sessionExpires = s.now().Add(30 * time.Minute)
	return cookie, nil
}

func (s *NineRouterSource) clearDashboardSession() {
	s.sessionMu.Lock()
	s.sessionCookie = ""
	s.sessionExpires = time.Time{}
	s.sessionMu.Unlock()
}

func (s *NineRouterSource) statusForModel(model UpstreamModel, snapshot nineRouterManagementSnapshot) ModelSourceStatus {
	provider := nineRouterProviderForModel(model.Upstream)
	status := ModelSourceStatus{
		SourceID: "9router", Source: "9Router", Kind: "9router",
		Provider: provider, Upstream: model.Upstream, State: SourceUnavailable,
		LastCheckedAt: snapshot.FetchedAt,
	}
	if provider == "" {
		status.State = SourceDegraded
		status.AvailableRoutes = 1
		status.TotalRoutes = 1
		status.LastError = "9Router provider mapping is unknown"
		return status
	}

	connections := make([]nineRouterConnection, 0, 2)
	for _, connection := range snapshot.Connections {
		if strings.EqualFold(strings.TrimSpace(connection.Provider), provider) {
			connections = append(connections, connection)
		}
	}
	status.TotalRoutes = len(connections)
	if len(connections) == 0 {
		status.LastError = "no 9Router connection is configured for this provider"
		return status
	}

	stateCounts := map[SourceState]int{}
	var bestPct *float64
	var bestReset time.Time
	var earliestRetry time.Time
	for _, connection := range connections {
		assessment := s.assessConnection(model, connection, snapshot)
		stateCounts[assessment.State]++
		if sourceUsable(ModelSourceStatus{State: assessment.State}) {
			status.AvailableRoutes++
		}
		if assessment.QuotaPct != nil && (bestPct == nil || *assessment.QuotaPct > *bestPct) {
			value := *assessment.QuotaPct
			bestPct = &value
			bestReset = assessment.ResetAt
		}
		candidateRetry := assessment.RetryAt
		if candidateRetry.IsZero() {
			candidateRetry = assessment.ResetAt
		}
		if !candidateRetry.IsZero() && (earliestRetry.IsZero() || candidateRetry.Before(earliestRetry)) {
			earliestRetry = candidateRetry
		}
	}
	status.QuotaRemainingPercent = bestPct
	status.QuotaResetAt = bestReset
	if status.AvailableRoutes > 0 {
		status.State = SourceHealthy
		if status.AvailableRoutes < status.TotalRoutes || stateCounts[SourceDegraded] > 0 {
			status.State = SourceDegraded
			status.LastError = "some 9Router routes are unavailable"
		}
		return status
	}

	status.RetryAt = earliestRetry
	switch {
	case stateCounts[SourceExhausted] > 0 && stateCounts[SourceExhausted] == status.TotalRoutes:
		status.State = SourceExhausted
		status.LastError = "all 9Router routes are quota exhausted"
	case stateCounts[SourceUnauthorized] > 0 && stateCounts[SourceUnauthorized] == status.TotalRoutes:
		status.State = SourceUnauthorized
		status.LastError = "all 9Router routes require re-authorization"
	case stateCounts[SourceCooldown] > 0:
		status.State = SourceCooldown
		status.LastError = "all usable 9Router routes are cooling down"
	case stateCounts[SourceExhausted] > 0:
		status.State = SourceExhausted
		status.LastError = "no 9Router route has remaining quota"
	case stateCounts[SourceUnauthorized] > 0:
		status.State = SourceUnauthorized
		status.LastError = "no authorized 9Router route is available"
	case stateCounts[SourceDisabled] == status.TotalRoutes:
		status.State = SourceDisabled
		status.LastError = "all 9Router routes are disabled"
	default:
		status.State = SourceUnavailable
		status.LastError = "no active 9Router route is available"
	}
	return status
}

func (s *NineRouterSource) assessConnection(model UpstreamModel, connection nineRouterConnection, snapshot nineRouterManagementSnapshot) nineRouterRouteAssessment {
	if !connection.IsActive {
		return nineRouterRouteAssessment{State: SourceDisabled, LastError: "connection disabled"}
	}
	testStatus := strings.ToLower(strings.TrimSpace(connection.TestStatus))
	if strings.Contains(testStatus, "unauthor") || strings.Contains(testStatus, "invalid") {
		return nineRouterRouteAssessment{State: SourceUnauthorized, LastError: "connection unauthorized"}
	}
	if testStatus == "unavailable" || testStatus == "offline" || testStatus == "failed" {
		return nineRouterRouteAssessment{State: SourceUnavailable, LastError: "connection unavailable"}
	}

	providerModel := nineRouterProviderModelID(model.Upstream)
	for _, unavailable := range snapshot.Availability {
		if unavailable.ConnectionID != connection.ID {
			continue
		}
		lockedModel := strings.TrimSpace(unavailable.Model)
		if lockedModel != "__all" && !strings.EqualFold(lockedModel, providerModel) && !strings.EqualFold(lockedModel, model.Upstream) && !strings.EqualFold(CanonicalModelID(lockedModel), model.Canonical) {
			continue
		}
		assessment := nineRouterRouteAssessment{State: SourceUnavailable, LastError: "route unavailable"}
		if strings.EqualFold(unavailable.Status, "cooldown") {
			assessment.State = SourceCooldown
			assessment.LastError = "route cooling down"
		}
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(unavailable.Until)); err == nil {
			assessment.RetryAt = parsed
		}
		return assessment
	}

	usage, hasUsage := snapshot.Usage[connection.ID]
	if hasUsage {
		if usage.Status == http.StatusUnauthorized || usage.Status == http.StatusForbidden {
			return nineRouterRouteAssessment{State: SourceUnauthorized, LastError: "quota session unauthorized"}
		}
		if usage.Err != nil || usage.Status >= 500 {
			return nineRouterRouteAssessment{State: SourceDegraded, LastError: "quota state unavailable"}
		}
		quota := assessNineRouterQuota(connection.Provider, providerModel, usage.Data)
		if quota.Known {
			assessment := nineRouterRouteAssessment{State: SourceHealthy, QuotaPct: quota.Percent, ResetAt: quota.ResetAt, QuotaKnown: true}
			if quota.Exhausted {
				assessment.State = SourceExhausted
				assessment.RetryAt = quota.ResetAt
				assessment.LastError = "quota exhausted"
			}
			return assessment
		}
	}
	return nineRouterRouteAssessment{State: SourceHealthy}
}

func assessNineRouterQuota(provider, providerModel string, usage map[string]any) nineRouterQuotaAssessment {
	if len(usage) == 0 {
		return nineRouterQuotaAssessment{}
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	model := strings.ToLower(strings.TrimSpace(providerModel))
	if provider == "codex" {
		flag := "limitReached"
		if strings.Contains(model, "review") {
			flag = "reviewLimitReached"
		} else if strings.Contains(model, "spark") {
			flag = "sparkLimitReached"
		}
		known := false
		if value, ok := boolValue(usage[flag]); ok {
			known = true
			if value {
				return nineRouterQuotaAssessment{Known: true, Exhausted: true}
			}
		}
		quotas := objectValue(usage["quotas"])
		selected := make([]map[string]any, 0, len(quotas))
		for key, raw := range quotas {
			lower := strings.ToLower(key)
			include := true
			switch {
			case strings.Contains(model, "spark"):
				include = strings.Contains(lower, "spark")
			case strings.Contains(model, "review"):
				include = strings.Contains(lower, "review")
			default:
				include = !strings.Contains(lower, "spark") && !strings.Contains(lower, "review")
			}
			if include {
				if item := objectValue(raw); item != nil {
					selected = append(selected, item)
				}
			}
		}
		if len(selected) > 0 {
			combined := combineIndependentQuotas(selected)
			combined.Known = combined.Known || known
			return combined
		}
		return nineRouterQuotaAssessment{Known: known}
	}

	quotas := objectValue(usage["quotas"])
	if len(quotas) == 0 {
		if value, ok := boolValue(usage["limitReached"]); ok {
			return nineRouterQuotaAssessment{Known: true, Exhausted: value}
		}
		return nineRouterQuotaAssessment{}
	}

	for key, raw := range quotas {
		if strings.EqualFold(strings.TrimSpace(key), providerModel) || strings.EqualFold(CanonicalModelID(key), CanonicalModelID(providerModel)) {
			if item := objectValue(raw); item != nil {
				return quotaObjectAssessment(item)
			}
		}
	}

	// CodeBuddy subscription packs are additive. A model remains usable while at
	// least one pack still has capacity.
	if provider == "codebuddy-intl" {
		items := make([]map[string]any, 0, len(quotas))
		for _, raw := range quotas {
			if item := objectValue(raw); item != nil {
				items = append(items, item)
			}
		}
		return combineAdditiveQuotas(items)
	}
	return nineRouterQuotaAssessment{}
}

func quotaObjectAssessment(item map[string]any) nineRouterQuotaAssessment {
	if unlimited, ok := boolValue(item["unlimited"]); ok && unlimited {
		return nineRouterQuotaAssessment{Known: true, Exhausted: false}
	}
	result := nineRouterQuotaAssessment{}
	if reset := stringValue(item["resetAt"]); reset != "" {
		if parsed, err := time.Parse(time.RFC3339, reset); err == nil {
			result.ResetAt = parsed
		}
	}
	if percent, ok := numberValue(item["remainingPercentage"]); ok {
		percent = clampPercent(percent)
		result.Known = true
		result.Percent = floatPointer(percent)
		result.Exhausted = percent <= 0
		return result
	}
	remaining, hasRemaining := numberValue(item["remaining"])
	total, hasTotal := numberValue(item["total"])
	if hasRemaining {
		result.Known = true
		result.Exhausted = remaining <= 0
		if hasTotal && total > 0 {
			result.Percent = floatPointer(clampPercent(remaining / total * 100))
		}
		return result
	}
	used, hasUsed := numberValue(item["used"])
	if hasUsed && hasTotal && total > 0 {
		result.Known = true
		remainingPct := clampPercent((total - used) / total * 100)
		result.Percent = floatPointer(remainingPct)
		result.Exhausted = used >= total
	}
	return result
}

func combineIndependentQuotas(items []map[string]any) nineRouterQuotaAssessment {
	result := nineRouterQuotaAssessment{}
	var lowest *float64
	for _, item := range items {
		assessment := quotaObjectAssessment(item)
		if !assessment.Known {
			continue
		}
		result.Known = true
		if assessment.Exhausted {
			result.Exhausted = true
		}
		if assessment.Percent != nil && (lowest == nil || *assessment.Percent < *lowest) {
			value := *assessment.Percent
			lowest = &value
			result.ResetAt = assessment.ResetAt
		}
	}
	result.Percent = lowest
	return result
}

func combineAdditiveQuotas(items []map[string]any) nineRouterQuotaAssessment {
	result := nineRouterQuotaAssessment{}
	allExhausted := true
	var highest *float64
	for _, item := range items {
		assessment := quotaObjectAssessment(item)
		if !assessment.Known {
			continue
		}
		result.Known = true
		if !assessment.Exhausted {
			allExhausted = false
		}
		if assessment.Percent != nil && (highest == nil || *assessment.Percent > *highest) {
			value := *assessment.Percent
			highest = &value
			result.ResetAt = assessment.ResetAt
		}
	}
	result.Exhausted = result.Known && allExhausted
	result.Percent = highest
	return result
}

func objectValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	if item, ok := value.(map[string]any); ok {
		return item
	}
	return nil
}

func boolValue(value any) (bool, bool) {
	item, ok := value.(bool)
	return item, ok
}

func numberValue(value any) (float64, bool) {
	switch item := value.(type) {
	case float64:
		return item, true
	case float32:
		return float64(item), true
	case int:
		return float64(item), true
	case int64:
		return float64(item), true
	case json.Number:
		value, err := item.Float64()
		return value, err == nil
	default:
		return 0, false
	}
}

func stringValue(value any) string {
	item, _ := value.(string)
	return strings.TrimSpace(item)
}

func floatPointer(value float64) *float64 { return &value }

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
