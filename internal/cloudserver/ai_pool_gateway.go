package cloudserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type dashboardAIPoolConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
}

func dashboardAIPoolCanonicalModelIDSafe(model string) bool {
	return dashboardModelIDSafe(model) && !strings.Contains(strings.TrimSpace(model), "/")
}

func dashboardAIPoolConfigFromEnv() (dashboardAIPoolConfig, bool) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CODELOCAL_AI_POOL_BASE_URL")), "/")
	apiKey := strings.TrimSpace(os.Getenv("CODELOCAL_AI_POOL_API_KEY"))
	if baseURL == "" || apiKey == "" {
		return dashboardAIPoolConfig{}, false
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return dashboardAIPoolConfig{}, false
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), "/v1") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	baseURL = strings.TrimRight(parsed.String(), "/")

	defaultModel := strings.TrimSpace(os.Getenv("CODELOCAL_AI_POOL_MODEL"))
	if defaultModel != "" && !dashboardAIPoolCanonicalModelIDSafe(defaultModel) {
		defaultModel = ""
	}

	return dashboardAIPoolConfig{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		DefaultModel: defaultModel,
	}, true
}

func dashboardAIPoolTarget(model string) (dashboardLLMTarget, bool) {
	config, ok := dashboardAIPoolConfigFromEnv()
	if !ok {
		return dashboardLLMTarget{}, false
	}
	model = strings.TrimSpace(model)
	if model == "" || model == dashboardModelAuto {
		model = config.DefaultModel
	}
	if !dashboardAIPoolCanonicalModelIDSafe(model) {
		return dashboardLLMTarget{}, false
	}
	return dashboardLLMTarget{
		ID:      "ai-pool:" + model,
		BaseURL: config.BaseURL,
		APIKey:  config.APIKey,
		Model:   model,
	}, true
}

func dashboardFetchAIPoolModels(ctx context.Context, config dashboardAIPoolConfig) ([]dashboardProviderModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(config.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, dashboardModelCatalogLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > dashboardModelCatalogLimit {
		return nil, errors.New("AI Pool model catalog response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	models := make([]dashboardProviderModel, 0, len(payload.Data))
	seen := make(map[string]bool, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if !dashboardAIPoolCanonicalModelIDSafe(id) || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, dashboardProviderModel{
			ID:                     id,
			OwnedBy:                strings.TrimSpace(item.OwnedBy),
			SupportedEndpointTypes: []string{"openai"},
		})
	}
	if len(models) == 0 {
		return nil, errors.New("AI Pool model catalog is empty")
	}
	return models, nil
}

func dashboardAIPoolModels(ctx context.Context) ([]dashboardProviderModel, error) {
	config, ok := dashboardAIPoolConfigFromEnv()
	if !ok {
		return nil, errors.New("AI Pool is not configured")
	}
	cacheKey := "ai-pool:" + dashboardModelCatalogCacheKey(config.BaseURL, config.APIKey)
	now := time.Now()

	dashboardModelCatalogCache.Lock()
	cached, hasCached := dashboardModelCatalogCache.Entries[cacheKey]
	if hasCached && now.Before(cached.ExpiresAt) {
		models := dashboardCopyProviderModels(cached.Models)
		dashboardModelCatalogCache.Unlock()
		return models, nil
	}
	dashboardModelCatalogCache.Unlock()

	models, err := dashboardFetchAIPoolModels(ctx, config)
	if err != nil {
		if hasCached && len(cached.Models) > 0 {
			return dashboardCopyProviderModels(cached.Models), nil
		}
		return nil, err
	}
	dashboardModelCatalogCache.Lock()
	dashboardModelCatalogCache.Entries[cacheKey] = dashboardModelCatalogEntry{
		Models:    dashboardCopyProviderModels(models),
		ExpiresAt: now.Add(dashboardModelCatalogTTL),
	}
	dashboardModelCatalogCache.Unlock()
	return models, nil
}

func dashboardAIPoolModelIDs(models []dashboardProviderModel, limit int) []string {
	if limit <= 0 || limit > len(models) {
		limit = len(models)
	}
	out := make([]string, 0, limit)
	for _, model := range models {
		if !dashboardModelSupportsChat(model) {
			continue
		}
		out = append(out, model.ID)
		if len(out) >= limit {
			break
		}
	}
	return out
}
