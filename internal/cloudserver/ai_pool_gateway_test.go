package cloudserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func clearAIPoolEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "")
}

func TestDashboardAIPoolConfigNormalizesGateway(t *testing.T) {
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "https://pool.example.test")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "codelocal-auto")

	config, ok := dashboardAIPoolConfigFromEnv()
	if !ok {
		t.Fatal("expected configured AI Pool")
	}
	if config.BaseURL != "https://pool.example.test/v1" {
		t.Fatalf("baseURL=%q", config.BaseURL)
	}
	if config.DefaultModel != "codelocal-auto" || config.APIKey != "pool-key" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestDashboardAIPoolTargetIsPrivateAndModelScoped(t *testing.T) {
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "https://pool.example.test/v1")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "codelocal-auto")

	target, ok := dashboardAIPoolTarget("")
	if !ok {
		t.Fatal("expected default Pool target")
	}
	if target.BaseURL != "https://pool.example.test/v1" || target.APIKey != "pool-key" || target.Model != "codelocal-auto" || target.Community {
		t.Fatalf("unexpected target: %#v", target)
	}
	if _, ok := dashboardAIPoolTarget("cc/claude-sonnet"); ok {
		t.Fatal("provider-qualified models must stay behind CodeLocal Pool")
	}
	canonical, ok := dashboardAIPoolTarget("claude-sonnet")
	if !ok || canonical.Model != "claude-sonnet" {
		t.Fatalf("canonical selections must be forwarded to Pool: %#v", canonical)
	}
}

func TestDashboardAIPoolCatalogMakesUnqualifiedComboRoutable(t *testing.T) {
	resetDashboardModelCatalogCache()
	t.Cleanup(resetDashboardModelCatalogCache)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer pool-key" {
			t.Fatalf("authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "premium-coding", "owned_by": "codelocal-pool"},
			{"id": "claude-sonnet", "owned_by": "codelocal-pool"},
			{"id": "cc/claude-sonnet", "owned_by": "should-never-leak"},
			{"id": "unsafe model", "owned_by": "bad"},
			{"id": "premium-coding", "owned_by": "codelocal-pool"},
		}})
	}))
	defer server.Close()

	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", server.URL+"/v1")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "codelocal-auto")

	models, err := dashboardAIPoolModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := dashboardAIPoolModelIDs(models, 0)
	want := []string{"premium-coding", "claude-sonnet"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models=%v want=%v", got, want)
	}
	selectable, err := dashboardSelectableModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantSelectable := []string{dashboardModelAuto, "premium-coding", "claude-sonnet"}
	if !reflect.DeepEqual(selectable, wantSelectable) {
		t.Fatalf("selectable=%v want=%v", selectable, wantSelectable)
	}
	route := dashboardLLMRoute("premium-coding", false)
	if len(route) != 1 || route[0].ID != "ai-pool:premium-coding" {
		t.Fatalf("discovered canonical model must route only through Pool: %#v", route)
	}
}
