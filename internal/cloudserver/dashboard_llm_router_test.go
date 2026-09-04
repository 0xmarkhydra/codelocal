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

func TestDashboardCommunityEligibility(t *testing.T) {
	if !dashboardCommunityEligible(dashboardChatRequest{Message: "xin chào"}) {
		t.Fatal("plain chat should be community eligible")
	}
	if dashboardCommunityEligible(dashboardChatRequest{Message: "token=secret"}) {
		t.Fatal("secret-like message must stay off community providers")
	}
	if dashboardCommunityEligible(dashboardChatRequest{Message: "xem project", Workspace: &dashboardChatWorkspace{WorkspaceID: "private"}}) {
		t.Fatal("workspace-bound chat must stay off community providers")
	}
	if dashboardCommunityEligible(dashboardChatRequest{Message: "xem ảnh", Image: "data:image/png;base64,AA=="}) {
		t.Fatal("image chat must stay off community providers")
	}
}

func TestDashboardLLMRouteOrder(t *testing.T) {
	clearAIPoolEnv(t)
	t.Setenv("OPENCODE_ZEN_API_KEY", "test-key")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "test-key")
	t.Setenv("CODELOCAL_LLM_BASE_URL", "https://opencode.ai/zen/v1")
	t.Setenv("CODELOCAL_LLM_MODEL", dashboardModelMuse)

	route := dashboardLLMRoute(dashboardModelAuto, true)
	if len(route) < 3 || route[0].Model != dashboardModelGLM || route[1].Model != dashboardModelQwen || route[2].Model != dashboardModelMuse {
		t.Fatalf("unexpected auto route: %#v", route)
	}

	privateRoute := dashboardLLMRoute(dashboardModelGLM, false)
	if len(privateRoute) != 0 {
		t.Fatalf("explicit community model must not fall back for private chat: %#v", privateRoute)
	}

	explicitRoute := dashboardLLMRoute(dashboardModelQwen, true)
	if len(explicitRoute) != 1 || explicitRoute[0].Model != dashboardModelQwen {
		t.Fatalf("explicit model route must be strict: %#v", explicitRoute)
	}
	privateMuseRoute := dashboardLLMRoute(dashboardModelMuse, false)
	if len(privateMuseRoute) != 0 {
		t.Fatalf("direct free Muse must not receive private context: %#v", privateMuseRoute)
	}
	privateAutoRoute := dashboardLLMRoute(dashboardModelAuto, false)
	if len(privateAutoRoute) != 0 {
		t.Fatalf("direct free fallbacks must not receive private context: %#v", privateAutoRoute)
	}
}

func TestDashboardAIPoolDefaultsAutoToMuseSpark13(t *testing.T) {
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "https://pool.example.test/v1")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_ENABLED", "1")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "")

	route := dashboardLLMRoute(dashboardModelAuto, false)
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

	route := dashboardLLMRoute(dashboardModelAuto, true)
	if len(route) != 1 || route[0].ID != "ai-pool:codelocal-auto" || route[0].Community {
		t.Fatalf("AI Pool must be the only auto target: %#v", route)
	}

	explicit := dashboardLLMRoute("gpt-5.6-sol", true)
	if len(explicit) != 1 || explicit[0].ID != "ai-pool:gpt-5.6-sol" || explicit[0].Community {
		t.Fatalf("canonical Pool selection should route strictly through Pool: %#v", explicit)
	}
	legacyNamed := dashboardLLMRoute(dashboardModelGLM, true)
	if len(legacyNamed) != 1 || legacyNamed[0].ID != "ai-pool:"+dashboardModelGLM {
		t.Fatalf("legacy-named selections must still go through Pool: %#v", legacyNamed)
	}
	providerQualified := dashboardLLMRoute("cc/claude-sonnet", false)
	if len(providerQualified) != 0 {
		t.Fatalf("provider-qualified ids must never be routable from CodeLocal UI: %#v", providerQualified)
	}
}
