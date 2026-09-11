package cloudserver

import (
	"context"
	"strings"
	"testing"
)

func TestDashboardSelectableModels(t *testing.T) {
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

func TestDashboardCommunityModelWithoutPrivateContextNeedsAllowCommunity(t *testing.T) {
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "zen-key")
	t.Setenv("OPENCODE_ZEN_API_KEY", "")
	if got := dashboardLLMRoute(dashboardModelMuse, false); len(got) != 0 {
		t.Fatalf("community muse with private context must have no route: %#v", got)
	}
	if got := dashboardLLMRoute(dashboardModelMuse, true); len(got) != 1 {
		t.Fatalf("community muse without private context must route: %#v", got)
	}
	if msg := dashboardCommunityBlockedMessage(); !strings.Contains(msg, "Auto") || !strings.Contains(msg, "workspace") {
		t.Fatalf("blocked message must guide the user: %q", msg)
	}
}

func TestDashboardCommunityWorkspaceOptIn(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_COMMUNITY_WORKSPACE", "")
	if dashboardCommunityWorkspaceAllowed() {
		t.Fatal("community workspace opt-in must default to off")
	}
	t.Setenv("CODELOCAL_ALLOW_COMMUNITY_WORKSPACE", "1")
	if !dashboardCommunityWorkspaceAllowed() {
		t.Fatal("community workspace opt-in must engage when set")
	}

	withWorkspace := dashboardChatRequest{
		Message:   "lam di",
		Workspace: &dashboardChatWorkspace{WorkspaceID: "codex-mcp"},
		Image:     "data:image/png;base64,AA==",
	}
	if dashboardCommunityEligible(withWorkspace) {
		t.Fatal("default lane must keep blocking workspace and image content")
	}
	if !dashboardCommunityOptInEligible(withWorkspace) {
		t.Fatal("opt-in lane must allow workspace and image content")
	}
	secretReq := dashboardChatRequest{Message: "deploy with password=hunter2"}
	if dashboardCommunityOptInEligible(secretReq) {
		t.Fatal("opt-in lane must keep blocking obvious secrets")
	}

	// End to end through the handler formula: explicit Muse plus attached
	// workspace routes once the deployment opts in.
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "zen-key")
	t.Setenv("OPENCODE_ZEN_API_KEY", "")
	allowCommunity := dashboardCommunityEligible(withWorkspace)
	if dashboardCommunityWorkspaceAllowed() {
		allowCommunity = dashboardCommunityOptInEligible(withWorkspace)
	}
	if !allowCommunity {
		t.Fatal("opt-in request must be community eligible")
	}
	if got := dashboardLLMRoute(dashboardModelMuse, allowCommunity); len(got) != 1 {
		t.Fatalf("opt-in muse route=%#v want one zen target", got)
	}
}

func TestDashboardVisionRoutingPrefersVisionTargets(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_COMMUNITY_WORKSPACE", "")
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "shop-key")
	t.Setenv("SHOPAIKEY_API_KEY", "")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	t.Setenv("CODELOCAL_SHOPAIKEY_MODEL", "gpt-5.6-sol")
	t.Setenv("OPENCODE_ZEN_API_KEY", "zen-key")

	if dashboardModelSupportsVision(dashboardModelMuse) {
		t.Fatal("community Muse must stay text-only for explicit vision routing")
	}
	if !dashboardModelSupportsVision("gpt-5.6-sol") {
		t.Fatal("gpt vision family must be vision-capable")
	}

	route := dashboardVisionRoute(dashboardModelAuto)
	if len(route) == 0 || !route[0].Vision || route[0].Model != "gpt-5.6-sol" {
		t.Fatalf("vision route must prefer Shop vision default: %#v", route)
	}
	for _, target := range route {
		if target.Community {
			t.Fatalf("vision route must never include community targets: %#v", route)
		}
	}

	if target, ok := dashboardChatVisionTarget(dashboardModelAuto, dashboardLLMRoute(dashboardModelAuto, false)); !ok || target.Model != "gpt-5.6-sol" {
		t.Fatalf("chat vision target must resolve the vision lane: %#v %v", target, ok)
	}
}

func TestDashboardVisionBlockedWithoutVisionLane(t *testing.T) {
	t.Setenv("CODELOCAL_SHOPAIKEY_API_KEY", "")
	t.Setenv("SHOPAIKEY_API_KEY", "")
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	t.Setenv("CODELOCAL_LLM_API_KEY", "zen-key")
	t.Setenv("OPENCODE_ZEN_API_KEY", "")

	if got := dashboardVisionRoute(dashboardModelAuto); len(got) != 0 {
		t.Fatalf("vision route without vision providers must be empty: %#v", got)
	}
	if msg := dashboardVisionBlockedMessage(); !strings.Contains(msg, "CODELOCAL_SHOPAIKEY_API_KEY") || strings.Contains(msg, "Pool") {
		t.Fatalf("vision blocked message must name the fix: %q", msg)
	}
}
