package cloudserver

import (
	"context"
	"testing"
)

func TestDashboardSelectableModels(t *testing.T) {
	clearAIPoolEnv(t)
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "")
	t.Setenv("SHOPAIKEY_API_KEY", "")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	models, err := dashboardSelectableModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{dashboardModelAuto, dashboardModelGLM, dashboardModelQwen, dashboardModelMuse}
	if len(models) != len(want) {
		t.Fatalf("models=%v want=%v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("models[%d]=%q want=%q", i, models[i], want[i])
		}
	}
}

func TestDashboardNormalizeModelSelection(t *testing.T) {
	cases := map[string]string{
		"":                       dashboardModelAuto,
		"Auto":                   dashboardModelAuto,
		"glm-5.3-flash":          dashboardModelGLM,
		"Qwen 3.8 Flash":         dashboardModelQwen,
		"Muse Spark 1.3":         dashboardModelMuse,
		"Muse Spark 1.2":         dashboardModelMuseLegacy,
		"unknown-provider-model": "unknown-provider-model",
		"../../unsafe model":     dashboardModelAuto,
	}
	for input, want := range cases {
		if got := dashboardNormalizeModelSelection(input); got != want {
			t.Fatalf("normalize(%q)=%q want=%q", input, got, want)
		}
	}
}

func TestDashboardSharedLaneBlocking(t *testing.T) {
	if dashboardSharedLaneBlocked(dashboardChatRequest{Message: "xin chào"}) {
		t.Fatal("plain chat should not be blocked")
	}
	if !dashboardSharedLaneBlocked(dashboardChatRequest{Message: "token=secret"}) {
		t.Fatal("secret-like message must stay off shared lanes")
	}
	// Model nào gửi ảnh model đó: image and workspace content must flow to
	// the selected model unchanged.
	if dashboardSharedLaneBlocked(dashboardChatRequest{Message: "xem project", Workspace: &dashboardChatWorkspace{WorkspaceID: "private"}}) {
		t.Fatal("workspace-bound chat must flow to the selected model")
	}
	if dashboardSharedLaneBlocked(dashboardChatRequest{Message: "xem ảnh", Image: "data:image/png;base64,AA=="}) {
		t.Fatal("image chat must flow to the selected model")
	}
}

func TestDashboardLLMRouteOrder(t *testing.T) {
	clearAIPoolEnv(t)
	t.Setenv("OPENCODE_ZEN_API_KEY", "test-key")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "test-key")
	t.Setenv("CODELOCAL_LLM_BASE_URL", "https://opencode.ai/zen/v1")
	t.Setenv("CODELOCAL_LLM_MODEL", dashboardModelMuse)

	route := dashboardLLMRoute(dashboardModelAuto)
	if len(route) < 3 || route[0].Model != dashboardModelGLM || route[1].Model != dashboardModelQwen || route[2].Model != dashboardModelMuse {
		t.Fatalf("unexpected auto route: %#v", route)
	}

	explicitRoute := dashboardLLMRoute(dashboardModelQwen)
	if len(explicitRoute) != 1 || explicitRoute[0].Model != dashboardModelQwen {
		t.Fatalf("explicit model route must be strict: %#v", explicitRoute)
	}
	explicitMuseRoute := dashboardLLMRoute(dashboardModelMuse)
	if len(explicitMuseRoute) != 1 || explicitMuseRoute[0].Model != dashboardModelMuse {
		t.Fatalf("explicit muse route must stay strict: %#v", explicitMuseRoute)
	}
}

func TestDashboardAIPoolDefaultsAutoToMuseSpark13(t *testing.T) {
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "https://pool.example.test/v1")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_ENABLED", "1")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "")

	route := dashboardLLMRoute(dashboardModelAuto)
	if len(route) != 1 || route[0].ID != "ai-pool:"+dashboardModelMuse || route[0].Model != dashboardModelMuse {
		t.Fatalf("Auto should default to Muse Spark 1.3 through Pool: %#v", route)
	}
}

func TestDashboardAIPoolRouteIsExclusiveWhenConfigured(t *testing.T) {
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "https://pool.example.test")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_ENABLED", "1")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "codelocal-auto")
	// Deliberately configure every legacy provider too. Pool must still be the
	// only execution plane once enabled.
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "shop-key")
	t.Setenv("SHOPAIKEY_API_KEY", "shop-key")
	t.Setenv("OPENCODE_ZEN_API_KEY", "zen-key")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "legacy-key")
	t.Setenv("CODELOCAL_LLM_BASE_URL", "https://legacy.example.test/v1")

	route := dashboardLLMRoute(dashboardModelAuto)
	if len(route) != 1 || route[0].ID != "ai-pool:codelocal-auto" {
		t.Fatalf("AI Pool must be the only auto target: %#v", route)
	}

	explicit := dashboardLLMRoute("gpt-5.6-sol")
	if len(explicit) != 1 || explicit[0].ID != "ai-pool:gpt-5.6-sol" {
		t.Fatalf("canonical Pool selection should route strictly through Pool: %#v", explicit)
	}
	legacyNamed := dashboardLLMRoute(dashboardModelGLM)
	if len(legacyNamed) != 1 || legacyNamed[0].ID != "ai-pool:"+dashboardModelGLM {
		t.Fatalf("legacy-named selections must still go through Pool: %#v", legacyNamed)
	}
	providerQualified := dashboardLLMRoute("cc/claude-sonnet")
	if len(providerQualified) != 0 {
		t.Fatalf("provider-qualified ids must never be routable from CodeLocal UI: %#v", providerQualified)
	}
}

func TestDashboardExplicitModelReceivesImageAndWorkspace(t *testing.T) {
	clearAIPoolEnv(t)
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "zen-key")
	t.Setenv("OPENCODE_ZEN_API_KEY", "")

	withImageWorkspace := dashboardChatRequest{
		Message:   "phân tích ảnh này",
		Workspace: &dashboardChatWorkspace{WorkspaceID: "codex-mcp"},
		Image:     "data:image/png;base64,AA==",
	}
	if dashboardSharedLaneBlocked(withImageWorkspace) {
		t.Fatal("image plus workspace request must stay routable for the selected model")
	}
	if got := dashboardLLMRoute(dashboardModelMuse); len(got) != 1 || got[0].Model != dashboardModelMuse {
		t.Fatalf("explicit muse with image+workspace must route strictly: %#v", got)
	}
}
