package aipool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeSourceResponse struct {
	status      int
	contentType string
	body        string
	headers     http.Header
	err         error
}

type fakeSource struct {
	id       string
	name     string
	kind     string
	priority int
	models   []UpstreamModel

	mu        sync.Mutex
	responses []fakeSourceResponse
	calls     int
}

func (s *fakeSource) ID() string    { return s.id }
func (s *fakeSource) Name() string  { return s.name }
func (s *fakeSource) Kind() string  { return s.kind }
func (s *fakeSource) Priority() int { return s.priority }

func (s *fakeSource) Models(context.Context) ([]UpstreamModel, error) {
	return append([]UpstreamModel(nil), s.models...), nil
}

func (s *fakeSource) Do(_ context.Context, _ *http.Request, _ string) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if len(s.responses) == 0 {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"model":"unknown","choices":[]}`)),
		}, nil
	}
	response := s.responses[0]
	if len(s.responses) > 1 {
		s.responses = s.responses[1:]
	}
	if response.err != nil {
		return nil, response.err
	}
	headers := response.headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	if response.contentType != "" {
		headers.Set("Content-Type", response.contentType)
	}
	return &http.Response{
		StatusCode: response.status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(response.body)),
	}, nil
}

func (s *fakeSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestCanonicalModelIDHides9RouterPrefixes(t *testing.T) {
	cases := map[string]string{
		"cx/gpt-5.6-sol":       "gpt-5.6-sol",
		"cc/claude-sonnet-4-6": "claude-sonnet-4-6",
		"gc/gemini-3.6-flash":  "gemini-3.6-flash",
		"gpt-5.6-sol":          "gpt-5.6-sol",
	}
	for input, want := range cases {
		if got := CanonicalModelID(input); got != want {
			t.Fatalf("CanonicalModelID(%q)=%q want=%q", input, got, want)
		}
	}
	if modelIDSafe("cx/gpt-5.6-sol") {
		t.Fatal("provider-qualified ids must not be valid public model ids")
	}
}

func TestPublicModelsExposeCanonicalActiveModelsOnly(t *testing.T) {
	source := &fakeSource{
		id: "9router", name: "9Router", kind: "9router", priority: 100,
		models: []UpstreamModel{
			{Canonical: "gpt-5.6-sol", Upstream: "cx/gpt-5.6-sol"},
			{Canonical: "gpt-5.5", Upstream: "cx/gpt-5.5"},
		},
	}
	server, err := NewServer(Config{APIKey: "pool-client-key", Registry: NewSourceRegistry(nil, source)})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer pool-client-key")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "cx/") {
		t.Fatalf("provider prefix leaked: %s", response.Body.String())
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 || payload.Data[0].ID != "gpt-5.5" || payload.Data[1].ID != "gpt-5.6-sol" {
		t.Fatalf("unexpected models: %+v", payload.Data)
	}
}

func TestRouterFailsOverSameCanonicalModelAndHidesOnlyWhenAllSourcesExhausted(t *testing.T) {
	byok := &fakeSource{
		id: "byok-a", name: "OpenAI BYOK", kind: "openai", priority: 10,
		models: []UpstreamModel{{Canonical: "gpt-5.6-sol", Upstream: "gpt-5.6-sol"}},
		responses: []fakeSourceResponse{{
			status:      http.StatusTooManyRequests,
			contentType: "application/json",
			body:        `{"error":{"message":"quota exhausted"}}`,
			headers:     http.Header{"Retry-After": []string{"3600"}},
		}},
	}
	nineRouter := &fakeSource{
		id: "9router", name: "9Router", kind: "9router", priority: 20,
		models: []UpstreamModel{{Canonical: "gpt-5.6-sol", Upstream: "cx/gpt-5.6-sol"}},
		responses: []fakeSourceResponse{
			{
				status:      http.StatusOK,
				contentType: "application/json",
				body:        `{"id":"chatcmpl-test","model":"cx/gpt-5.6-sol","choices":[{"message":{"role":"assistant","content":"ok"}}]}`,
			},
			{
				status:      http.StatusTooManyRequests,
				contentType: "application/json",
				body:        `{"error":{"message":"codex quota exhausted"}}`,
				headers:     http.Header{"Retry-After": []string{"3600"}},
			},
		},
	}
	router := NewRouter(NewSourceRegistry(nil, byok, nineRouter))

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstOut := httptest.NewRecorder()
	if err := router.Route(firstOut, firstReq); err != nil {
		t.Fatal(err)
	}
	if byok.callCount() != 1 || nineRouter.callCount() != 1 {
		t.Fatalf("expected BYOK then 9Router fallback, calls byok=%d 9router=%d", byok.callCount(), nineRouter.callCount())
	}
	if strings.Contains(firstOut.Body.String(), "cx/") || !strings.Contains(firstOut.Body.String(), `"model":"gpt-5.6-sol"`) {
		t.Fatalf("canonical response was not preserved: %s", firstOut.Body.String())
	}

	active, err := router.Models(context.Background(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "gpt-5.6-sol" || active[0].AvailableSources != 1 || active[0].TotalSources != 2 {
		t.Fatalf("model should remain active while 9Router is healthy: %+v", active)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"again"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondOut := httptest.NewRecorder()
	if err := router.Route(secondOut, secondReq); err == nil {
		t.Fatal("expected all sources to become exhausted")
	}
	if byok.callCount() != 1 {
		t.Fatalf("exhausted BYOK source should be skipped, calls=%d", byok.callCount())
	}
	if nineRouter.callCount() != 2 {
		t.Fatalf("9Router should receive second attempt, calls=%d", nineRouter.callCount())
	}

	active, err = router.Models(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("canonical model must disappear only after every source is exhausted: %+v", active)
	}
	all, err := router.Models(context.Background(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "gpt-5.6-sol" || all[0].Active || all[0].State != "exhausted" || all[0].AvailableSources != 0 {
		t.Fatalf("admin catalog should retain exhausted model: %+v", all)
	}
}

func TestRouterRewritesProviderModelInSSE(t *testing.T) {
	source := &fakeSource{
		id: "9router", name: "9Router", kind: "9router", priority: 10,
		models: []UpstreamModel{{Canonical: "gpt-5.6-sol", Upstream: "cx/gpt-5.6-sol"}},
		responses: []fakeSourceResponse{{
			status:      http.StatusOK,
			contentType: "text/event-stream",
			body:        "data: {\"id\":\"x\",\"model\":\"cx/gpt-5.6-sol\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n",
		}},
	}
	router := NewRouter(NewSourceRegistry(nil, source))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	out := httptest.NewRecorder()
	if err := router.Route(out, req); err != nil {
		t.Fatal(err)
	}
	body := out.Body.String()
	if strings.Contains(body, "cx/") || !strings.Contains(body, `"model":"gpt-5.6-sol"`) || !strings.Contains(body, "[DONE]") {
		t.Fatalf("provider prefix leaked from SSE: %s", body)
	}
}

func TestRouterRejectsProviderQualifiedClientModel(t *testing.T) {
	source := &fakeSource{
		id: "9router", name: "9Router", kind: "9router", priority: 10,
		models: []UpstreamModel{{Canonical: "gpt-5.6-sol", Upstream: "cx/gpt-5.6-sol"}},
	}
	router := NewRouter(NewSourceRegistry(nil, source))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"cx/gpt-5.6-sol"}`))
	out := httptest.NewRecorder()
	if err := router.Route(out, req); err == nil || !strings.Contains(err.Error(), "provider-qualified") {
		t.Fatalf("expected provider-qualified model rejection, err=%v", err)
	}
}

func TestServerRequiresExactClientBearerKey(t *testing.T) {
	source := &fakeSource{id: "9router", name: "9Router", kind: "9router", models: []UpstreamModel{{Canonical: "gpt-5.6-sol", Upstream: "cx/gpt-5.6-sol"}}}
	server, err := NewServer(Config{APIKey: "pool-key", Registry: NewSourceRegistry(nil, source)})
	if err != nil {
		t.Fatal(err)
	}
	for name, auth := range map[string]struct {
		value string
		want  int
	}{
		"missing": {"", http.StatusUnauthorized},
		"wrong":   {"Bearer pool-key-extra", http.StatusUnauthorized},
		"exact":   {"Bearer pool-key", http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if auth.value != "" {
				req.Header.Set("Authorization", auth.value)
			}
			out := httptest.NewRecorder()
			server.Handler().ServeHTTP(out, req)
			if out.Code != auth.want {
				t.Fatalf("status=%d want=%d body=%s", out.Code, auth.want, out.Body.String())
			}
		})
	}
}
