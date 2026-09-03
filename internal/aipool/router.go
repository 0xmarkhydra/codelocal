package aipool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type routeCandidate struct {
	Source   Source
	Model    UpstreamModel
	Status   ModelSourceStatus
	stateKey string
}

type sourceCatalog struct {
	Models    []UpstreamModel
	FetchedAt time.Time
}

type Router struct {
	registry   *SourceRegistry
	catalogTTL time.Duration
	now        func() time.Time

	mu       sync.Mutex
	catalogs map[string]sourceCatalog
	states   map[string]ModelSourceStatus
}

const (
	routeTransientAttempts      = 3
	nineRouterRateLimitAttempts = 2
	nineRouterRuntimeCooldown   = 500 * time.Millisecond
)

func NewRouter(registry *SourceRegistry) *Router {
	return &Router{
		registry:   registry,
		catalogTTL: 15 * time.Second,
		now:        time.Now,
		catalogs:   map[string]sourceCatalog{},
		states:     map[string]ModelSourceStatus{},
	}
}

func routeStateKey(sourceID, canonical, upstream string) string {
	return sourceID + "\x00" + canonical + "\x00" + upstream
}

func (r *Router) sourceModels(ctx context.Context, source Source) ([]UpstreamModel, error) {
	now := r.now()
	r.mu.Lock()
	cached, ok := r.catalogs[source.ID()]
	if ok && now.Sub(cached.FetchedAt) < r.catalogTTL {
		models := append([]UpstreamModel(nil), cached.Models...)
		r.mu.Unlock()
		return models, nil
	}
	r.mu.Unlock()

	models, err := source.Models(ctx)
	if err != nil {
		if ok && len(cached.Models) > 0 {
			return append([]UpstreamModel(nil), cached.Models...), nil
		}
		return nil, err
	}
	r.mu.Lock()
	r.catalogs[source.ID()] = sourceCatalog{Models: append([]UpstreamModel(nil), models...), FetchedAt: now}
	r.mu.Unlock()
	return models, nil
}

func baseStatusFor(source Source, model UpstreamModel) ModelSourceStatus {
	if model.Status != nil {
		status := *model.Status
		status.SourceID = source.ID()
		status.Source = source.Name()
		status.Kind = source.Kind()
		if status.Upstream == "" {
			status.Upstream = model.Upstream
		}
		if status.TotalRoutes <= 0 {
			status.TotalRoutes = 1
		}
		if sourceUsable(status) && status.AvailableRoutes <= 0 {
			status.AvailableRoutes = 1
		}
		return status
	}
	return ModelSourceStatus{
		SourceID: source.ID(), Source: source.Name(), Kind: source.Kind(), Upstream: model.Upstream,
		State: SourceHealthy, AvailableRoutes: 1, TotalRoutes: 1,
	}
}

func (r *Router) statusFor(source Source, model UpstreamModel) ModelSourceStatus {
	base := baseStatusFor(source, model)
	key := routeStateKey(source.ID(), model.Canonical, model.Upstream)
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.states[key]
	if !ok {
		return base
	}
	if !status.RetryAt.IsZero() && !r.now().Before(status.RetryAt) {
		delete(r.states, key)
		return base
	}
	status.SourceID = source.ID()
	status.Source = source.Name()
	status.Kind = source.Kind()
	status.Provider = base.Provider
	status.Upstream = model.Upstream
	status.AvailableRoutes = 0
	status.TotalRoutes = base.TotalRoutes
	status.QuotaRemainingPercent = base.QuotaRemainingPercent
	status.QuotaResetAt = base.QuotaResetAt
	status.LastCheckedAt = base.LastCheckedAt
	return status
}

func sourceUsable(status ModelSourceStatus) bool {
	switch status.State {
	case SourceHealthy, SourceDegraded:
		return true
	default:
		return false
	}
}

func inactiveModelStateRank(state string) int {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "cooldown":
		return 4
	case "unauthorized":
		return 3
	case "exhausted":
		return 2
	case "unavailable":
		return 1
	default:
		return 0
	}
}

func (r *Router) sources(ctx context.Context) ([]Source, error) {
	if r.registry == nil {
		return nil, errors.New("Pool source registry is not configured")
	}
	return r.registry.Sources(ctx)
}

func (r *Router) candidates(ctx context.Context, canonical string) ([]routeCandidate, error) {
	sources, err := r.sources(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []routeCandidate
	var lastErr error
	for _, source := range sources {
		models, modelErr := r.sourceModels(ctx, source)
		if modelErr != nil {
			lastErr = modelErr
			continue
		}
		for _, model := range models {
			if model.Canonical != canonical {
				continue
			}
			status := r.statusFor(source, model)
			candidates = append(candidates, routeCandidate{
				Source:   source,
				Model:    model,
				Status:   status,
				stateKey: routeStateKey(source.ID(), model.Canonical, model.Upstream),
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Source.Priority() != candidates[j].Source.Priority() {
			return candidates[i].Source.Priority() < candidates[j].Source.Priority()
		}
		return candidates[i].Source.ID() < candidates[j].Source.ID()
	})
	if len(candidates) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return candidates, nil
}

// Models returns canonical models. activeOnly is used by /v1/models; admin
// surfaces pass false so exhausted models remain visible for diagnosis.
func (r *Router) Models(ctx context.Context, activeOnly, detailed bool) ([]CanonicalModel, error) {
	sources, err := r.sources(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]*CanonicalModel{}
	sourceSeen := map[string]map[string]bool{}
	sourceAvailable := map[string]map[string]bool{}
	var lastErr error
	for _, source := range sources {
		models, modelErr := r.sourceModels(ctx, source)
		if modelErr != nil {
			lastErr = modelErr
			continue
		}
		for _, upstream := range models {
			if !modelIDSafe(upstream.Canonical) {
				continue
			}
			model := byID[upstream.Canonical]
			if model == nil {
				model = &CanonicalModel{ID: upstream.Canonical, State: "unavailable"}
				byID[upstream.Canonical] = model
			}
			status := r.statusFor(source, upstream)
			if sourceSeen[upstream.Canonical] == nil {
				sourceSeen[upstream.Canonical] = map[string]bool{}
				sourceAvailable[upstream.Canonical] = map[string]bool{}
			}
			if !sourceSeen[upstream.Canonical][source.ID()] {
				sourceSeen[upstream.Canonical][source.ID()] = true
				model.TotalSources++
			}
			totalRoutes := status.TotalRoutes
			if totalRoutes <= 0 {
				totalRoutes = 1
			}
			availableRoutes := status.AvailableRoutes
			if sourceUsable(status) && availableRoutes <= 0 {
				availableRoutes = 1
			}
			model.TotalRoutes += totalRoutes
			model.AvailableRoutes += availableRoutes
			if sourceUsable(status) {
				if !sourceAvailable[upstream.Canonical][source.ID()] {
					sourceAvailable[upstream.Canonical][source.ID()] = true
					model.AvailableSources++
				}
				model.Active = true
				if status.State == SourceHealthy {
					model.State = "active"
				} else if model.State != "active" {
					model.State = "degraded"
				}
			} else if !model.Active {
				candidateState := string(status.State)
				if status.State == SourceDisabled {
					candidateState = "unavailable"
				}
				if inactiveModelStateRank(candidateState) > inactiveModelStateRank(model.State) {
					model.State = candidateState
				}
			}
			if detailed {
				model.Sources = append(model.Sources, status)
			}
		}
	}
	out := make([]CanonicalModel, 0, len(byID))
	for _, model := range byID {
		if activeOnly && !model.Active {
			continue
		}
		out = append(out, *model)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID) })
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

func retryAfterAt(now time.Time, resp *http.Response, fallback time.Duration) time.Time {
	if resp != nil {
		value := strings.TrimSpace(resp.Header.Get("Retry-After"))
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			return now.Add(time.Duration(seconds) * time.Second)
		}
		if parsed, err := http.ParseTime(value); err == nil {
			return parsed
		}
		for _, header := range []string{"X-RateLimit-Reset", "X-RateLimit-Reset-Requests"} {
			value = strings.TrimSpace(resp.Header.Get(header))
			if unix, err := strconv.ParseInt(value, 10, 64); err == nil && unix > now.Unix() {
				return time.Unix(unix, 0)
			}
		}
	}
	return now.Add(fallback)
}

func routeRetryDelay(attempt int) time.Duration {
	delay := 150 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

func waitRouteRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(routeRetryDelay(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func routeRetryableNetworkError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func routeRetryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

type sourceHealthInvalidator interface {
	InvalidateHealth()
}

func (r *Router) invalidateCandidateHealth(candidate routeCandidate) {
	if invalidator, ok := candidate.Source.(sourceHealthInvalidator); ok {
		invalidator.InvalidateHealth()
	}
	r.mu.Lock()
	delete(r.catalogs, candidate.Source.ID())
	r.mu.Unlock()
}

func (r *Router) mark(candidate routeCandidate, state SourceState, retryAt time.Time, errText string) {
	r.mu.Lock()
	r.states[candidate.stateKey] = ModelSourceStatus{
		SourceID: candidate.Source.ID(), Source: candidate.Source.Name(), Kind: candidate.Source.Kind(),
		State: state, RetryAt: retryAt, LastError: errText,
	}
	r.mu.Unlock()
}

func (r *Router) healthy(candidate routeCandidate) {
	r.mu.Lock()
	delete(r.states, candidate.stateKey)
	r.mu.Unlock()
}

func cloneRequestWithBody(in *http.Request, body []byte) (*http.Request, error) {
	out, err := http.NewRequestWithContext(in.Context(), in.Method, in.URL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	out.Header = in.Header.Clone()
	return out, nil
}

func canonicalRequestModel(body []byte) (string, error) {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	payload.Model = strings.TrimSpace(payload.Model)
	if isProviderQualifiedModel(payload.Model) {
		return "", errors.New("provider-qualified model ids are not accepted by CodeLocal Pool")
	}
	if !modelIDSafe(payload.Model) {
		return "", errors.New("invalid model")
	}
	return payload.Model, nil
}

// Route proxies an OpenAI-compatible request while preserving the requested
// canonical model. Failover only changes source/upstream representation; it
// never silently changes the model requested by the client.
func (r *Router) Route(w http.ResponseWriter, req *http.Request) error {
	if req.Body == nil {
		return errors.New("request body required")
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 16<<20))
	if err != nil {
		return err
	}
	canonical, err := canonicalRequestModel(body)
	if err != nil {
		return err
	}
	candidates, err := r.candidates(req.Context(), canonical)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return &ModelUnavailableError{Model: canonical}
	}

	var lastErr error
	var lastStatus int
	for _, candidate := range candidates {
		if !sourceUsable(candidate.Status) {
			continue
		}
	CandidateAttempts:
		for attemptIndex := 0; attemptIndex < routeTransientAttempts; attemptIndex++ {
			attempt, cloneErr := cloneRequestWithBody(req, body)
			if cloneErr != nil {
				return cloneErr
			}
			resp, requestErr := candidate.Source.Do(req.Context(), attempt, candidate.Model.Upstream)
			if requestErr != nil {
				lastErr = requestErr
				if attemptIndex < routeTransientAttempts-1 && routeRetryableNetworkError(req.Context(), requestErr) {
					if waitErr := waitRouteRetry(req.Context(), attemptIndex); waitErr != nil {
						return waitErr
					}
					continue
				}
				cooldown := 30 * time.Second
				if candidate.Source.Kind() == "9router" {
					cooldown = nineRouterRuntimeCooldown
					r.invalidateCandidateHealth(candidate)
				}
				r.mark(candidate, SourceCooldown, r.now().Add(cooldown), "network error")
				break CandidateAttempts
			}

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				r.healthy(candidate)
				defer resp.Body.Close()
				copyResponseHeaders(w.Header(), resp.Header)
				w.WriteHeader(resp.StatusCode)
				return copyCanonicalResponse(w, resp, canonical)
			}

			lastStatus = resp.StatusCode
			errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else {
				lastErr = errors.New(strings.TrimSpace(string(errorBody)))
			}
			now := r.now()
			switch resp.StatusCode {
			case http.StatusTooManyRequests:
				if candidate.Source.Kind() == "9router" {
					r.invalidateCandidateHealth(candidate)
					if attemptIndex < nineRouterRateLimitAttempts-1 {
						if waitErr := waitRouteRetry(req.Context(), attemptIndex); waitErr != nil {
							return waitErr
						}
						continue
					}
					// 9Router owns connection-level quota and failover. A single inference
					// 429 must not quarantine the whole canonical model for ten minutes;
					// force a management refresh on the next request instead.
					r.mark(candidate, SourceCooldown, now.Add(nineRouterRuntimeCooldown), "9Router rate limit; health refresh required")
					break CandidateAttempts
				}
				r.mark(candidate, SourceExhausted, retryAfterAt(now, resp, 10*time.Minute), "quota/rate limit")
				break CandidateAttempts
			case http.StatusUnauthorized, http.StatusForbidden:
				r.mark(candidate, SourceUnauthorized, now.Add(30*time.Minute), "credential rejected")
				break CandidateAttempts
			default:
				if routeRetryableStatus(resp.StatusCode) {
					if attemptIndex < routeTransientAttempts-1 {
						if waitErr := waitRouteRetry(req.Context(), attemptIndex); waitErr != nil {
							return waitErr
						}
						continue
					}
					cooldown := 30 * time.Second
					if candidate.Source.Kind() == "9router" {
						cooldown = nineRouterRuntimeCooldown
						r.invalidateCandidateHealth(candidate)
					}
					r.mark(candidate, SourceCooldown, now.Add(cooldown), "upstream unavailable")
					break CandidateAttempts
				}
				copyResponseHeaders(w.Header(), resp.Header)
				w.WriteHeader(resp.StatusCode)
				_, _ = w.Write(errorBody)
				return nil
			}
		}
	}
	return &UpstreamsUnavailableError{Model: canonical, Status: lastStatus, Err: lastErr}
}

func (r *Router) TestModel(ctx context.Context, canonical string) (ModelTestResult, error) {
	canonical = strings.TrimSpace(canonical)
	result := ModelTestResult{Model: canonical}
	if isProviderQualifiedModel(canonical) || !modelIDSafe(canonical) {
		return result, errors.New("invalid canonical model")
	}
	candidates, err := r.candidates(ctx, canonical)
	if err != nil {
		return result, err
	}
	if len(candidates) == 0 {
		result.Error = "model is not available in the Pool catalog"
		return result, nil
	}

	payload, err := json.Marshal(map[string]any{
		"model":    canonical,
		"messages": []map[string]string{{"role": "user", "content": "Reply exactly OK."}},
		"stream":   false,
	})
	if err != nil {
		return result, err
	}

	for _, candidate := range candidates {
		if !sourceUsable(candidate.Status) {
			continue
		}
		result.Attempts++
		result.Source = candidate.Source.Name()
		result.Provider = candidate.Status.Provider
		if result.Provider == "" {
			result.Provider = candidate.Source.Kind()
		}
		result.Upstream = candidate.Model.Upstream

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://pool.local/v1/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return result, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		started := time.Now()
		resp, err := candidate.Source.Do(ctx, req, candidate.Model.Upstream)
		result.LatencyMS = time.Since(started).Milliseconds()
		if err != nil {
			r.mark(candidate, SourceCooldown, r.now().Add(30*time.Second), "probe request failed")
			result.Error = "upstream request failed"
			continue
		}
		result.HTTPStatus = resp.StatusCode
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.healthy(candidate)
			result.OK = true
			result.Error = ""
			return result, nil
		}

		now := r.now()
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			r.mark(candidate, SourceExhausted, retryAfterAt(now, resp, 10*time.Minute), "quota/rate limit")
			result.Error = "quota or rate limit reached"
			continue
		case http.StatusUnauthorized, http.StatusForbidden:
			r.mark(candidate, SourceUnauthorized, now.Add(30*time.Minute), "credential rejected")
			result.Error = "upstream credential rejected"
			continue
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			r.mark(candidate, SourceCooldown, now.Add(30*time.Second), "upstream unavailable")
			result.Error = "upstream temporarily unavailable"
			continue
		default:
			result.Error = fmt.Sprintf("upstream returned status %d", resp.StatusCode)
			return result, nil
		}
	}
	if result.Attempts == 0 {
		result.Error = "no usable route is currently available"
	}
	return result, nil
}

func copyCanonicalResponse(w io.Writer, resp *http.Response, canonical string) error {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		return rewriteSSEModel(w, resp.Body, canonical)
	}
	if strings.Contains(contentType, "application/json") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			return err
		}
		var payload any
		if json.Unmarshal(body, &payload) == nil {
			rewriteModelFields(payload, canonical)
			rewritten, marshalErr := json.Marshal(payload)
			if marshalErr == nil {
				_, err = w.Write(rewritten)
				return err
			}
		}
		_, err = w.Write(body)
		return err
	}
	_, err := io.Copy(w, resp.Body)
	return err
}

func rewriteModelFields(value any, canonical string) {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["model"]; ok {
			typed["model"] = canonical
		}
		for _, child := range typed {
			rewriteModelFields(child, canonical)
		}
	case []any:
		for _, child := range typed {
			rewriteModelFields(child, canonical)
		}
	}
}

func rewriteSSEModel(w io.Writer, body io.Reader, canonical string) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" && data != "[DONE]" {
				var payload any
				if json.Unmarshal([]byte(data), &payload) == nil {
					rewriteModelFields(payload, canonical)
					if rewritten, err := json.Marshal(payload); err == nil {
						line = "data: " + string(rewritten)
					}
				}
			}
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
		if flusher, ok := w.(interface{ Flush() }); ok {
			flusher.Flush()
		}
	}
	return scanner.Err()
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		switch strings.ToLower(key) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "content-length":
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

type ModelUnavailableError struct{ Model string }

func (e *ModelUnavailableError) Error() string { return "model unavailable: " + e.Model }

type UpstreamsUnavailableError struct {
	Model  string
	Status int
	Err    error
}

func (e *UpstreamsUnavailableError) Error() string {
	if e.Err != nil {
		return "all sources unavailable for " + e.Model + ": " + e.Err.Error()
	}
	return "all sources unavailable for " + e.Model
}

func (e *UpstreamsUnavailableError) Unwrap() error { return e.Err }
