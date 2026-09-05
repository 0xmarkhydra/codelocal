package cloudserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	dashboardShopAIKeyBaseURL  = "https://api.shopaikey.com/v1"
	dashboardOpenRouterModels  = "https://openrouter.ai/api/v1/models"
	dashboardModelCatalogTTL   = 5 * time.Minute
	dashboardModelRankingTTL   = 30 * time.Minute
	dashboardModelCatalogLimit = 4 << 20
	dashboardPopularModelLimit = 20
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

type dashboardModelRankingEntry struct {
	Models    []string
	ExpiresAt time.Time
}

var dashboardModelCatalogCache = struct {
	sync.Mutex
	Entries map[string]dashboardModelCatalogEntry
}{Entries: map[string]dashboardModelCatalogEntry{}}

var dashboardModelRankingCache = struct {
	sync.Mutex
	Entries map[string]dashboardModelRankingEntry
}{Entries: map[string]dashboardModelRankingEntry{}}

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
		// Shop chat models accept OpenAI image_url blocks on the general chat
		// completions lane; explicit text-only community sparklines never flow
		// through this target.
		Vision: dashboardModelSupportsVision(model),
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

func dashboardCopyModelIDs(models []string) []string {
	out := make([]string, len(models))
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

func dashboardOpenRouterModelsURL() string {
	if endpoint := strings.TrimSpace(os.Getenv("CODELOCAL_OPENROUTER_MODELS_URL")); endpoint != "" {
		return endpoint
	}
	return dashboardOpenRouterModels
}

func dashboardFetchOpenRouterPopularModels(ctx context.Context, endpoint string) ([]string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("output_modalities", "text")
	query.Set("sort", "most-popular")
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
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
		return nil, errors.New("OpenRouter model ranking response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if dashboardModelIDSafe(model.ID) {
			models = append(models, model.ID)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("OpenRouter model ranking is empty")
	}
	return models, nil
}

func dashboardOpenRouterPopularModels(ctx context.Context) ([]string, error) {
	endpoint := dashboardOpenRouterModelsURL()
	now := time.Now()

	dashboardModelRankingCache.Lock()
	cached, hasCached := dashboardModelRankingCache.Entries[endpoint]
	if hasCached && now.Before(cached.ExpiresAt) {
		models := dashboardCopyModelIDs(cached.Models)
		dashboardModelRankingCache.Unlock()
		return models, nil
	}
	dashboardModelRankingCache.Unlock()

	models, err := dashboardFetchOpenRouterPopularModels(ctx, endpoint)
	if err != nil {
		if hasCached && len(cached.Models) > 0 {
			return dashboardCopyModelIDs(cached.Models), nil
		}
		return nil, err
	}
	dashboardModelRankingCache.Lock()
	dashboardModelRankingCache.Entries[endpoint] = dashboardModelRankingEntry{
		Models:    dashboardCopyModelIDs(models),
		ExpiresAt: now.Add(dashboardModelRankingTTL),
	}
	dashboardModelRankingCache.Unlock()
	return models, nil
}

func dashboardModelPopularityKey(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = model[slash+1:]
	}
	if variant := strings.Index(model, ":"); variant >= 0 {
		model = model[:variant]
	}
	var normalized strings.Builder
	separator := false
	for _, char := range model {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			if separator && normalized.Len() > 0 {
				normalized.WriteByte('-')
			}
			normalized.WriteRune(char)
			separator = false
			continue
		}
		separator = true
	}
	return normalized.String()
}

func dashboardModelsRankedByOpenRouter(providerModels []dashboardProviderModel, rankedModels []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	available := make([]string, 0, len(providerModels))
	for _, model := range providerModels {
		if dashboardModelSupportsChat(model) {
			available = append(available, model.ID)
		}
	}
	sort.Slice(available, func(i, j int) bool {
		return strings.ToLower(available[i]) < strings.ToLower(available[j])
	})
	byPopularityKey := make(map[string]string, len(available))
	for _, model := range available {
		key := dashboardModelPopularityKey(model)
		if key != "" {
			if _, exists := byPopularityKey[key]; !exists {
				byPopularityKey[key] = model
			}
		}
	}

	selected := make([]string, 0, limit)
	seen := make(map[string]bool, limit)
	for _, rankedModel := range rankedModels {
		model := byPopularityKey[dashboardModelPopularityKey(rankedModel)]
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		selected = append(selected, model)
		if len(selected) == limit {
			break
		}
	}
	return selected
}

func dashboardUniqueModels(groups ...[]string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, group := range groups {
		for _, model := range group {
			model = strings.TrimSpace(model)
			if !dashboardModelIDSafe(model) || seen[model] {
				continue
			}
			seen[model] = true
			out = append(out, model)
		}
	}
	return out
}

func dashboardPoolSelectableModels(ctx context.Context) []string {
	if _, ok := dashboardAIPoolConfigFromEnv(); !ok {
		return nil
	}
	models, err := dashboardAIPoolModels(ctx)
	if err != nil {
		return nil
	}
	// Pool already exposes only canonical models that have at least one usable
	// route. Do not apply the generic "popular 20" cap: the CodeLocal picker must
	// reflect the complete active Pool catalog so an explicit user selection is
	// always routable.
	return dashboardAIPoolModelIDs(models, 0)
}

func dashboardCuratedModels(ctx context.Context) []string {
	if _, ok := dashboardAIPoolConfigFromEnv(); ok {
		return dashboardUniqueModels(
			[]string{dashboardModelAuto},
			dashboardPoolSelectableModels(ctx),
		)
	}
	return dashboardUniqueModels(
		[]string{dashboardModelAuto},
		[]string{dashboardModelGLM, dashboardModelQwen, dashboardModelMuse},
	)
}

func dashboardSelectableModels(ctx context.Context) ([]string, error) {
	// Pool configured means Pool owns the complete CodeLocal chat model catalog.
	// Do not mix legacy/provider-specific models into the picker, otherwise a user
	// can select a model that bypasses Pool and breaks the single control-plane
	// contract.
	if _, ok := dashboardAIPoolConfigFromEnv(); ok {
		models, err := dashboardAIPoolModels(ctx)
		if err != nil {
			return []string{dashboardModelAuto}, err
		}
		return dashboardUniqueModels(
			[]string{dashboardModelAuto},
			dashboardAIPoolModelIDs(models, 0),
		), nil
	}

	// Temporary direct-Zen lane: an explicitly configured Zen credential pins
	// the picker to Auto + Muse Spark 1.3 so chat stays on the single
	// temporary model instead of the ShopAIKey/curated catalogs.
	if dashboardZenLanePinned() {
		return []string{dashboardModelAuto, dashboardModelMuse}, nil
	}

	if _, _, _, ok := dashboardShopAIKeyConfig(); !ok {
		return dashboardCuratedModels(ctx), nil
	}
	providerModels, err := dashboardShopAIKeyModels(ctx)
	if err != nil {
		return []string{dashboardModelAuto}, err
	}
	rankedModels, err := dashboardOpenRouterPopularModels(ctx)
	if err != nil {
		return []string{dashboardModelAuto}, err
	}
	models := dashboardModelsRankedByOpenRouter(providerModels, rankedModels, dashboardPopularModelLimit)
	if len(models) == 0 {
		return []string{dashboardModelAuto}, errors.New("no popular OpenRouter models are available through ShopAIKey")
	}
	return dashboardUniqueModels([]string{dashboardModelAuto}, models), nil
}
