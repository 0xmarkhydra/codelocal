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
		"Muse Spark 1.2":         dashboardModelMuse,
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
}

func TestDashboardAIPoolRouteIsPrivateAndFirstForAuto(t *testing.T) {
	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", "https://pool.example.test")
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "pool-key")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "codelocal-auto")
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "")
	t.Setenv("SHOPAIKEY_API_KEY", "")
	t.Setenv("OPENCODE_ZEN_API_KEY", "")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	t.Setenv("CODELOCAL_LLM_API_KEY", "")

	route := dashboardLLMRoute(dashboardModelAuto, false)
	if len(route) == 0 || route[0].ID != "ai-pool:codelocal-auto" || route[0].Community {
		t.Fatalf("AI Pool should be the first private auto target: %#v", route)
	}

	explicit := dashboardLLMRoute("cc/claude-sonnet", false)
	if len(explicit) != 1 || explicit[0].ID != "ai-pool:cc/claude-sonnet" || explicit[0].Community {
		t.Fatalf("provider-qualified Pool selection should route strictly through Pool: %#v", explicit)
	}
}
