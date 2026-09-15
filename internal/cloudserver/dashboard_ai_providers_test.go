package cloudserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardUserModelSelectionRoundTrip(t *testing.T) {
	selection := dashboardUserModelSelection("abcDEF_1234567890", "openai/gpt-5.6-sol")
	providerID, model, ok := dashboardParseUserModelSelection(selection)
	if !ok || providerID != "abcDEF_1234567890" || model != "openai/gpt-5.6-sol" {
		t.Fatalf("selection round trip failed: provider=%q model=%q ok=%v", providerID, model, ok)
	}
	if got := dashboardNormalizeModelSelection(selection); got != selection {
		t.Fatalf("user model selection normalized away: %q", got)
	}
	for _, bad := range []string{"byo::gpt-5", "byo:short:gpt-5", "byo:abcDEF_1234567890:bad model", "byo:abc/unsafe:gpt-5"} {
		if _, _, ok := dashboardParseUserModelSelection(bad); ok {
			t.Fatalf("invalid user model selection accepted: %q", bad)
		}
	}
}

func TestDashboardUserProviderRouteIsSticky(t *testing.T) {
	selection := dashboardUserModelSelection("abcDEF_1234567890", "gpt-5.6-sol")
	target := dashboardLLMTarget{ID: selection, BaseURL: "https://api.example.com/v1", APIKeyBytes: []byte("secret"), Model: "gpt-5.6-sol", Vision: true}
	ctx := context.WithValue(context.Background(), dashboardUserProviderRouteKey{}, dashboardUserProviderRoute{Selection: selection, Target: target})
	route := dashboardLLMRouteWithContext(ctx, selection, false, true)
	if len(route) != 1 || route[0].ID != selection || route[0].BaseURL != target.BaseURL {
		t.Fatalf("user-owned selection must remain pinned to its provider: %#v", route)
	}
}

func TestDashboardMergeModelSelectionsKeepsLongUserOwnedSelection(t *testing.T) {
	model := strings.Repeat("m", 150)
	selection := dashboardUserModelSelection("abcDEF_1234567890", model)
	if len(selection) <= 160 {
		t.Fatalf("test selection must exceed the legacy model-id cap: %d", len(selection))
	}
	merged := dashboardMergeModelSelections([]string{"auto"}, []string{selection})
	if len(merged) != 2 || merged[1] != selection {
		t.Fatalf("valid user-owned selection was filtered out: %#v", merged)
	}
}

func TestDashboardProviderNetworkSafeRejectsPrivateTargetsWithoutDialing(t *testing.T) {
	for _, raw := range []string{"https://127.0.0.1/v1", "https://10.20.30.40/v1", "https://192.168.1.2/v1"} {
		if err := dashboardProviderNetworkSafe(context.Background(), raw); err == nil {
			t.Fatalf("private provider target accepted: %s", raw)
		}
	}
}

func TestDashboardLLMHTTPClientDoesNotFollowRedirects(t *testing.T) {
	finalCalled := false
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	resp, err := dashboardLLMHTTPClient(0).Get(redirect.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound || finalCalled {
		t.Fatalf("LLM client followed redirect: status=%d finalCalled=%v", resp.StatusCode, finalCalled)
	}
}
