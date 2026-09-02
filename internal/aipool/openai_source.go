package aipool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OpenAISourceOptions struct {
	ID            string
	Name          string
	Kind          string
	BaseURL       string
	APIKey        string
	Priority      int
	StripPrefixes bool
}

type OpenAISource struct {
	id            string
	name          string
	kind          string
	baseURL       string
	apiKey        string
	priority      int
	stripPrefixes bool
	client        *http.Client
}

func NewOpenAISource(opts OpenAISourceOptions) (*OpenAISource, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" || strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("source base URL and API key are required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
		return nil, errors.New("invalid source base URL")
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), "/v1") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		return nil, errors.New("source id is required")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = id
	}
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = "openai-compatible"
	}
	priority := opts.Priority
	if priority < 0 {
		priority = 0
	}
	return &OpenAISource{
		id:            id,
		name:          name,
		kind:          kind,
		baseURL:       strings.TrimRight(parsed.String(), "/"),
		apiKey:        strings.TrimSpace(opts.APIKey),
		priority:      priority,
		stripPrefixes: opts.StripPrefixes,
		client: &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 90 * time.Second,
		}},
	}, nil
}

func (s *OpenAISource) ID() string      { return s.id }
func (s *OpenAISource) Name() string    { return s.name }
func (s *OpenAISource) Kind() string    { return s.kind }
func (s *OpenAISource) Priority() int   { return s.priority }
func (s *OpenAISource) BaseURL() string { return s.baseURL }

func (s *OpenAISource) Models(ctx context.Context) ([]UpstreamModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("source model catalog unavailable")
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	models := make([]UpstreamModel, 0, len(payload.Data))
	seen := map[string]bool{}
	for _, item := range payload.Data {
		upstream := strings.TrimSpace(item.ID)
		canonical := upstream
		if s.stripPrefixes {
			canonical = CanonicalModelID(upstream)
		}
		if !modelIDSafe(canonical) || upstream == "" || len(upstream) > 200 || seen[canonical+"\x00"+upstream] {
			continue
		}
		seen[canonical+"\x00"+upstream] = true
		models = append(models, UpstreamModel{Canonical: canonical, Upstream: upstream})
	}
	if len(models) == 0 {
		return nil, errors.New("source model catalog is empty")
	}
	return models, nil
}

func (s *OpenAISource) Do(ctx context.Context, incoming *http.Request, upstreamModel string) (*http.Response, error) {
	if incoming == nil || incoming.Body == nil {
		return nil, errors.New("request is required")
	}
	body, err := io.ReadAll(io.LimitReader(incoming.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["model"] = upstreamModel
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	path := strings.TrimSpace(incoming.URL.Path)
	if strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	out, err := http.NewRequestWithContext(ctx, incoming.Method, s.baseURL+path, bytes.NewReader(rewritten))
	if err != nil {
		return nil, err
	}
	for key, values := range incoming.Header {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "authorization", "proxy-authorization", "cookie", "x-csrf-token", "host", "content-length":
			// Client/session credentials belong to the Pool boundary and must never
			// be forwarded to a provider. The source gets only its own credential.
			continue
		}
		for _, value := range values {
			out.Header.Add(key, value)
		}
	}
	out.Header.Set("Authorization", "Bearer "+s.apiKey)
	out.Header.Set("Content-Type", "application/json")
	return s.client.Do(out)
}
