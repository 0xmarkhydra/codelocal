package cloudserver

import "testing"

func TestDashboardSelectableModels(t *testing.T) {
	models := dashboardSelectableModels()
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
		"unknown-provider-model": dashboardModelAuto,
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
	if len(privateRoute) == 0 || privateRoute[0].Model != dashboardModelMuse || privateRoute[0].Community {
		t.Fatalf("private route should start on trusted Muse: %#v", privateRoute)
	}
}
