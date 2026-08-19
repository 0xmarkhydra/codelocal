package cloudserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNextDashboardRoutingExcludesAdmin(t *testing.T) {
	for _, path := range []string{"/dashboard", "/dashboard/devices", "/dashboard/knowledge", "/dashboard/account"} {
		if !isNextDashboardPath(path) {
			t.Fatalf("expected %q to use Next dashboard", path)
		}
	}
	for _, path := range []string{"/dashboard/admin", "/dashboard/admin/users", "/api/v1/account", "/client", "/mcp"} {
		if isNextDashboardPath(path) {
			t.Fatalf("expected %q to stay on Go", path)
		}
	}
}

func TestWebFrontendProxyRequiresExplicitCutoverAndValidOrigin(t *testing.T) {
	t.Setenv("CODELOCAL_WEB_CUTOVER", "0")
	t.Setenv("CODELOCAL_WEB_ORIGIN", "https://example.com")
	proxy, err := newWebFrontendProxyFromEnv()
	if err != nil || proxy != nil {
		t.Fatalf("disabled cutover returned proxy=%v err=%v", proxy != nil, err)
	}

	t.Setenv("CODELOCAL_WEB_CUTOVER", "1")
	for _, origin := range []string{"", "ftp://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?query=1"} {
		t.Setenv("CODELOCAL_WEB_ORIGIN", origin)
		if proxy, err := newWebFrontendProxyFromEnv(); err == nil || proxy != nil {
			t.Fatalf("invalid origin %q unexpectedly accepted", origin)
		}
	}
}

func TestWebFrontendProxyStripsBrowserSecretsAndClientIPSignals(t *testing.T) {
	type observedHeaders struct {
		cookie, authorization, realIP, forwardedFor string
	}
	observed := make(chan observedHeaders, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- observedHeaders{
			cookie: r.Header.Get("Cookie"), authorization: r.Header.Get("Authorization"),
			realIP: r.Header.Get("X-Real-IP"), forwardedFor: r.Header.Get("X-Forwarded-For"),
		}
		_, _ = io.WriteString(w, "next")
	}))
	defer upstream.Close()

	t.Setenv("CODELOCAL_WEB_CUTOVER", "1")
	t.Setenv("CODELOCAL_WEB_ORIGIN", upstream.URL)
	proxy, err := newWebFrontendProxyFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://codelocal.test/dashboard", nil)
	request.Header.Set("Cookie", "codelocal_session=secret-session")
	request.Header.Set("Authorization", "Bearer private-token")
	request.Header.Set("X-Real-IP", "198.51.100.7")
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "next" {
		t.Fatalf("unexpected proxy response: status=%d body=%q", response.Code, response.Body.String())
	}
	got := <-observed
	if got.cookie != "" || got.authorization != "" || got.realIP != "" || got.forwardedFor != "" {
		t.Fatalf("presentation proxy leaked browser security headers: %#v", got)
	}
}
