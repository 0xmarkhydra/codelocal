package cloudserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	dashboardShopAIKeyBaseURL  = "https://api.shopaikey.com/v1"
	dashboardModelCatalogTTL   = 5 * time.Minute
	dashboardModelCatalogLimit = 4 << 20
)

type dashboardProviderModel struct {
	ID                     string   `json:"id"`
	OwnedBy                string   `json:"owned_by"`
	SupportedEndpointTypes []string `json:"supported_endpoint_types"`
}

type dashboardModelCatalogEntry struct {
	Models    []dashboardProviderModel
	ExpiresAt time.Time
}

var dashboardModelCatalogCache = struct {
	sync.Mutex
	Entries map[string]dashboardModelCatalogEntry
}{Entries: map[string]dashboardModelCatalogEntry{}}

func dashboardShopAIKeyConfig() (apiKey, baseURL, defaultModel string, ok bool) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_LLM_PROVIDER")))
	apiKey = strings.TrimSpace(os.Getenv("CODELOCAL_SHOPAIKEY_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("SHOPAIKEY_API_KEY"))
	}
	if apiKey == "" && provider == "shopaikey" {
		apiKey = strings.TrimSpace(os.Getenv("CODELOCAL_LLM_API_KEY"))
	}
	if apiKey == "" {
		return "", "", "", false
	}

	baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("CODELOCAL_SHOPAIKEY_BASE_URL")), "/")
	if baseURL == "" && provider == "shopaikey" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("CODELOCAL_LLM_BASE_URL")), "/")
	}
	if baseURL == "" {
		baseURL = dashboardShopAIKeyBaseURL
	}

	defaultModel = strings.TrimSpace(os.Getenv("CODELOCAL_SHOPAIKEY_MODEL"))
	if defaultModel == "" && provider == "shopaikey" {
		defaultModel = strings.TrimSpace(os.Getenv("CODELOCAL_LLM_MODEL"))
	}
	if !dashboardModelIDSafe(defaultModel) {
		defaultModel = "qwen3.5-flash"
	}
	return apiKey, baseURL, defaultModel, true
}

func dashboardShopAIKeyTarget(model string) (dashboardLLMTarget, bool) {
	apiKey, baseURL, defaultModel, ok := dashboardShopAIKeyConfig()
	if !ok {
		return dashboardLLMTarget{}, false
	}
	model = strings.TrimSpace(model)
	if model == "" || model == dashboardModelAuto {
		model = defaultModel
	}
	if !dashboardModelIDSafe(model) {
		return dashboardLLMTarget{}, false
	}
	return dashboardLLMTarget{
		ID:      "shopaikey:" + model,
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
	}, true
}

func dashboardModelIDSafe(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 160 {
		return false
	}
	for _, char := range model {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case strings.ContainsRune("-._:/", char):
		default:
			return false
		}
	}
	return true
}

func dashboardModelSupportsChat(model dashboardProviderModel) bool {
	if !dashboardModelIDSafe(model.ID) {
		return false
	}
	hasOpenAIEndpoint := false
	for _, endpointType := range model.SupportedEndpointTypes {
		if strings.EqualFold(strings.TrimSpace(endpointType), "openai") {
			hasOpenAIEndpoint = true
			break
		}
	}
	if !hasOpenAIEndpoint {
		return false
	}

	// /v1/models contains media, embedding and reranking models as well as chat
	// models. Those model families use different request/response contracts and
	// must not be offered by the text/vision chat composer.
	lower := strings.ToLower(model.ID)
	for _, marker := range []string{
		"image", "embedding", "rerank", "moderation", "seedream",
		"speech", "transcrib", "tts", "whisper", "audio", "video", "suno",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func dashboardModelCatalogCacheKey(baseURL, apiKey string) string {
	digest := sha256.Sum256([]byte(apiKey))
	return strings.TrimRight(baseURL, "/") + ":" + hex.EncodeToString(digest[:8])
}

func dashboardCopyProviderModels(models []dashboardProviderModel) []dashboardProviderModel {
	out := make([]dashboardProviderModel, len(models))
	copy(out, models)
	return out
}

func dashboardFetchShopAIKeyModels(ctx context.Context, baseURL, apiKey string) ([]dashboardProviderModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
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
		return nil, errors.New("model catalog response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	var payload struct {
		Data []dashboardProviderModel `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("model catalog is empty")
	}
	return payload.Data, nil
}

func dashboardShopAIKeyModels(ctx context.Context) ([]dashboardProviderModel, error) {
	apiKey, baseURL, _, ok := dashboardShopAIKeyConfig()
	if !ok {
		return nil, errors.New("ShopAIKey is not configured")
	}
	cacheKey := dashboardModelCatalogCacheKey(baseURL, apiKey)
	now := time.Now()

	dashboardModelCatalogCache.Lock()
	cached, hasCached := dashboardModelCatalogCache.Entries[cacheKey]
	if hasCached && now.Before(cached.ExpiresAt) {
		models := dashboardCopyProviderModels(cached.Models)
		dashboardModelCatalogCache.Unlock()
		return models, nil
	}
	dashboardModelCatalogCache.Unlock()

	models, err := dashboardFetchShopAIKeyModels(ctx, baseURL, apiKey)
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

func dashboardCuratedModels() []string {
	return []string{dashboardModelAuto, dashboardModelGLM, dashboardModelQwen, dashboardModelMuse}
}

func dashboardSelectableModels(ctx context.Context) ([]string, error) {
	if _, _, _, ok := dashboardShopAIKeyConfig(); !ok {
		return dashboardCuratedModels(), nil
	}
	providerModels, err := dashboardShopAIKeyModels(ctx)
	if err != nil {
		return dashboardCuratedModels(), err
	}
	seen := map[string]bool{dashboardModelAuto: true}
	models := make([]string, 0, len(providerModels)+1)
	for _, model := range providerModels {
		if !dashboardModelSupportsChat(model) || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		models = append(models, model.ID)
	}
	sort.Slice(models, func(i, j int) bool {
		return strings.ToLower(models[i]) < strings.ToLower(models[j])
	})
	return append([]string{dashboardModelAuto}, models...), nil
}
