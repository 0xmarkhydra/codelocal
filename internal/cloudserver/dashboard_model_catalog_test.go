package cloudserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func resetDashboardModelCatalogCache() {
	dashboardModelCatalogCache.Lock()
	dashboardModelCatalogCache.Entries = map[string]dashboardModelCatalogEntry{}
	dashboardModelCatalogCache.Unlock()
}

func TestDashboardModelSupportsChat(t *testing.T) {
	cases := []struct {
		model dashboardProviderModel
		want  bool
	}{
		{model: dashboardProviderModel{ID: "gpt-5.4", SupportedEndpointTypes: []string{"openai"}}, want: true},
		{model: dashboardProviderModel{ID: "claude-sonnet-4-6", SupportedEndpointTypes: []string{"anthropic", "openai"}}, want: true},
		{model: dashboardProviderModel{ID: "gpt-image-1", SupportedEndpointTypes: []string{"image-generation", "openai"}}, want: false},
		{model: dashboardProviderModel{ID: "text-embedding-3-small", SupportedEndpointTypes: []string{"openai"}}, want: false},
		{model: dashboardProviderModel{ID: "unsafe model", SupportedEndpointTypes: []string{"openai"}}, want: false},
		{model: dashboardProviderModel{ID: "claude-only", SupportedEndpointTypes: []string{"anthropic"}}, want: false},
	}
	for _, test := range cases {
		if got := dashboardModelSupportsChat(test.model); got != test.want {
			t.Fatalf("dashboardModelSupportsChat(%q)=%v want=%v", test.model.ID, got, test.want)
		}
	}
}

func TestDashboardSelectableModelsFromShopAIKey(t *testing.T) {
	resetDashboardModelCatalogCache()
	t.Cleanup(resetDashboardModelCatalogCache)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-shop-key" {
			t.Fatalf("authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "qwen3.5-flash", "owned_by": "custom", "supported_endpoint_types": []string{"openai"}},
			{"id": "gpt-image-1", "owned_by": "openai", "supported_endpoint_types": []string{"image-generation", "openai"}},
			{"id": "Claude-Sonnet-4-6", "owned_by": "custom", "supported_endpoint_types": []string{"anthropic", "openai"}},
			{"id": "anthropic-only", "owned_by": "custom", "supported_endpoint_types": []string{"anthropic"}},
		}})
	}))
	defer server.Close()

	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "test-shop-key")
	t.Setenv("CODELOCAL_SHOPAIKEY_BASE_URL", server.URL+"/v1")
	models, err := dashboardSelectableModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"auto", "Claude-Sonnet-4-6", "qwen3.5-flash"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models=%v want=%v", models, want)
	}
}

func TestDashboardShopAIKeySelectedModelRoutesFirst(t *testing.T) {
	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "test-shop-key")
	t.Setenv("CODELOCAL_SHOPAIKEY_BASE_URL", "https://shop.example.test/v1")
	t.Setenv("CODELOCAL_SHOPAIKEY_MODEL", "qwen3.5-flash")

	direct := dashboardLLMRoute("claude-sonnet-4-6", false)
	if len(direct) != 1 || direct[0].BaseURL != "https://shop.example.test/v1" || direct[0].Model != "claude-sonnet-4-6" || direct[0].Community {
		t.Fatalf("direct route=%#v", direct)
	}
	auto := dashboardLLMRoute(dashboardModelAuto, false)
	if len(auto) == 0 || auto[0].Model != "qwen3.5-flash" {
		t.Fatalf("auto route=%#v", auto)
	}
}

func TestDashboardShopAIKeySelectedModelRetriesWithoutFallback(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, `{"error":"temporarily unavailable"}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "test-shop-key")
	t.Setenv("CODELOCAL_SHOPAIKEY_BASE_URL", server.URL)
	t.Setenv("CODELOCAL_SHOPAIKEY_MODEL", "qwen3.5-flash")

	target, _, _, err := callDashboardLLMWithTools("claude-opus-5", false, []map[string]any{{"role": "user", "content": "hello"}}, nil)
	if err == nil {
		t.Fatal("expected selected model error")
	}
	var selectedErr *dashboardSelectedModelError
	if !errors.As(err, &selectedErr) {
		t.Fatalf("error=%T %v, want dashboardSelectedModelError", err, err)
	}
	if requests != 2 {
		t.Fatalf("requests=%d, want one initial attempt plus one same-model retry", requests)
	}
	if target.Model != "" {
		t.Fatalf("unexpected fallback target: %#v", target)
	}
	if got := dashboardFriendlyStreamError(err); got != selectedErr.Error() {
		t.Fatalf("friendly error=%q want=%q", got, selectedErr.Error())
	}
}
