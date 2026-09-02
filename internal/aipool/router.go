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

func (r *Router) statusFor(source Source, model UpstreamModel) ModelSourceStatus {
	key := routeStateKey(source.ID(), model.Canonical, model.Upstream)
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.states[key]
	if !ok {
		return ModelSourceStatus{SourceID: source.ID(), Source: source.Name(), Kind: source.Kind(), State: SourceHealthy}
	}
	if !status.RetryAt.IsZero() && !r.now().Before(status.RetryAt) {
		delete(r.states, key)
		return ModelSourceStatus{SourceID: source.ID(), Source: source.Name(), Kind: source.Kind(), State: SourceHealthy}
	}
	status.SourceID = source.ID()
	status.Source = source.Name()
	status.Kind = source.Kind()
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
				model = &CanonicalModel{ID: upstream.Canonical, State: "exhausted"}
				byID[upstream.Canonical] = model
			}
			status := r.statusFor(source, upstream)
			model.TotalSources++
			if sourceUsable(status) {
				model.AvailableSources++
				model.Active = true
				if status.State == SourceDegraded && model.State != "active" {
					model.State = "degraded"
				} else {
					model.State = "active"
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
		attempt, err := cloneRequestWithBody(req, body)
		if err != nil {
			return err
		}
		resp, err := candidate.Source.Do(req.Context(), attempt, candidate.Model.Upstream)
		if err != nil {
			lastErr = err
			var netErr net.Error
			if errors.As(err, &netErr) {
				r.mark(candidate, SourceCooldown, r.now().Add(30*time.Second), "network error")
			} else {
				r.mark(candidate, SourceCooldown, r.now().Add(15*time.Second), "upstream request failed")
			}
			continue
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
			r.mark(candidate, SourceExhausted, retryAfterAt(now, resp, 10*time.Minute), "quota/rate limit")
			continue
		case http.StatusUnauthorized, http.StatusForbidden:
			r.mark(candidate, SourceUnauthorized, now.Add(30*time.Minute), "credential rejected")
			continue
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			r.mark(candidate, SourceCooldown, now.Add(30*time.Second), "upstream unavailable")
			continue
		default:
			copyResponseHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(errorBody)
			return nil
		}
	}
	return &UpstreamsUnavailableError{Model: canonical, Status: lastStatus, Err: lastErr}
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
