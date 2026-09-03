package aipool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type nineRouterFixture struct {
	models       []string
	connections  []nineRouterConnection
	availability []nineRouterAvailability
	usage        map[string]map[string]any
}

func newNineRouterFixtureServer(t *testing.T, fixture nineRouterFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/auth/login" && r.Method == http.MethodPost:
			var input map[string]string
			if json.NewDecoder(r.Body).Decode(&input) != nil || input["password"] != "admin-password" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "auth_token", Value: "fixture-session", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.URL.Path == "/v1/models":
			if r.Header.Get("Authorization") != "Bearer endpoint-key" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			data := make([]map[string]string, 0, len(fixture.models))
			for _, model := range fixture.models {
				data = append(data, map[string]string{"id": model})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
		case r.URL.Path == "/api/providers":
			if !strings.Contains(r.Header.Get("Cookie"), "auth_token=fixture-session") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"connections": fixture.connections})
		case r.URL.Path == "/api/models/availability":
			if !strings.Contains(r.Header.Get("Cookie"), "auth_token=fixture-session") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": fixture.availability})
		case strings.HasPrefix(r.URL.Path, "/api/usage/"):
			if !strings.Contains(r.Header.Get("Cookie"), "auth_token=fixture-session") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id := strings.TrimPrefix(r.URL.Path, "/api/usage/")
			payload, ok := fixture.usage[id]
			if !ok {
				payload = map[string]any{"message": "Usage API not implemented"}
			}
			_ = json.NewEncoder(w).Encode(payload)
		default:
			http.NotFound(w, r)
		}
	}))
}

func fixtureNineRouterSource(t *testing.T, server *httptest.Server) *NineRouterSource {
	t.Helper()
	source, err := NewNineRouterSource(NineRouterSourceOptions{
		BaseURL:       server.URL + "/v1",
		APIKey:        "endpoint-key",
		AdminPassword: "admin-password",
		Priority:      100,
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestNineRouterTwoCodexConnectionsKeepModelUsableWhileOneHasQuota(t *testing.T) {
	server := newNineRouterFixtureServer(t, nineRouterFixture{
		models: []string{"cx/gpt-5.6-sol"},
		connections: []nineRouterConnection{
			{ID: "codex-a", Provider: "codex", TestStatus: "active", IsActive: true, Priority: 1},
			{ID: "codex-b", Provider: "codex", TestStatus: "active", IsActive: true, Priority: 2},
		},
		usage: map[string]map[string]any{
			"codex-a": {
				"limitReached": true,
				"quotas":       map[string]any{"session": map[string]any{"used": 100.0, "total": 100.0, "remaining": 0.0}},
			},
			"codex-b": {
				"limitReached": false,
				"quotas":       map[string]any{"session": map[string]any{"used": 50.0, "total": 100.0, "remaining": 50.0}},
			},
		},
	})
	defer server.Close()

	router := NewRouter(NewSourceRegistry(nil, fixtureNineRouterSource(t, server)))
	models, err := router.Models(context.Background(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models=%+v", models)
	}
	model := models[0]
	if model.ID != "gpt-5.6-sol" || !model.Active || model.State != "degraded" {
		t.Fatalf("model=%+v", model)
	}
	if model.AvailableRoutes != 1 || model.TotalRoutes != 2 {
		t.Fatalf("route counts=%d/%d", model.AvailableRoutes, model.TotalRoutes)
	}
	if len(model.Sources) != 1 || model.Sources[0].QuotaRemainingPercent == nil || *model.Sources[0].QuotaRemainingPercent != 50 {
		t.Fatalf("source status=%+v", model.Sources)
	}
}

func TestNineRouterAntigravityModelQuotaCanExhaustCanonicalModel(t *testing.T) {
	server := newNineRouterFixtureServer(t, nineRouterFixture{
		models: []string{"ag/gemini-3.6-flash-low"},
		connections: []nineRouterConnection{
			{ID: "ag-a", Provider: "antigravity", TestStatus: "active", IsActive: true, Priority: 1},
		},
		usage: map[string]map[string]any{
			"ag-a": {
				"quotas": map[string]any{
					"gemini-3.6-flash-low": map[string]any{"used": 1000.0, "total": 1000.0, "remainingPercentage": 0.0},
				},
			},
		},
	})
	defer server.Close()

	router := NewRouter(NewSourceRegistry(nil, fixtureNineRouterSource(t, server)))
	active, err := router.Models(context.Background(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("exhausted model leaked into active catalog: %+v", active)
	}
	all, err := router.Models(context.Background(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "gemini-3.6-flash-low" || all[0].Active || all[0].State != "exhausted" {
		t.Fatalf("model=%+v", all)
	}
}

func TestNineRouterAvailabilityCooldownOverridesHealthyQuota(t *testing.T) {
	server := newNineRouterFixtureServer(t, nineRouterFixture{
		models: []string{"cx/gpt-5.4"},
		connections: []nineRouterConnection{
			{ID: "codex-a", Provider: "codex", TestStatus: "active", IsActive: true, Priority: 1},
		},
		availability: []nineRouterAvailability{
			{Provider: "codex", Model: "__all", Status: "cooldown", ConnectionID: "codex-a"},
		},
		usage: map[string]map[string]any{
			"codex-a": {
				"limitReached": false,
				"quotas":       map[string]any{"session": map[string]any{"used": 10.0, "total": 100.0, "remaining": 90.0}},
			},
		},
	})
	defer server.Close()

	router := NewRouter(NewSourceRegistry(nil, fixtureNineRouterSource(t, server)))
	active, err := router.Models(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("cooldown model leaked into active catalog: %+v", active)
	}
	all, err := router.Models(context.Background(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].State != "cooldown" || len(all[0].Sources) != 1 || all[0].Sources[0].State != SourceCooldown {
		t.Fatalf("model=%+v", all)
	}
}
