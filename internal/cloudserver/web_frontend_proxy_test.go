package cloudserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNextDashboardRoutingUsesCompletePresentationWhitelist(t *testing.T) {
	for _, path := range []string{
		"/dashboard",
		"/dashboard/connect",
		"/dashboard/devices",
		"/dashboard/workspaces",
		"/dashboard/knowledge",
		"/dashboard/code-graph",
		"/dashboard/usage",
		"/dashboard/invite",
		"/dashboard/leaderboard",
		"/dashboard/security",
		"/dashboard/account",
		"/dashboard/admin",
	} {
		if !isNextDashboardPath(path) {
			t.Fatalf("expected %q to use Next dashboard", path)
		}
	}
	for _, path := range []string{
		"/dashboard/admin/users",
		"/dashboard/unknown",
		"/api/v1/account",
		"/client",
		"/mcp",
	} {
		if isNextDashboardPath(path) {
			t.Fatalf("expected %q to stay on Go", path)
		}
	}
}

func TestNextPresentationOwnsPublicAndFreshSecurityPages(t *testing.T) {
	for _, path := range []string{"/", "/login", "/register", "/signup", "/signup/verify", "/forgot-password", "/reset-password", "/privacy", "/terms", "/support", "/security", "/healthz"} {
		if !isNextPublicPagePath(path) {
			t.Fatalf("expected %q to be a Next public page", path)
		}
	}
	for _, path := range []string{"/authorize", "/pair/approve"} {
		if !isNextFreshSecurityPath(path) {
			t.Fatalf("expected %q to require fresh-security Next presentation", path)
		}
	}
}

func TestNextPresentationProxyNeverOwnsMutations(t *testing.T) {
	if !isNextPresentationMethod(http.MethodGet) || !isNextPresentationMethod(http.MethodHead) {
		t.Fatal("GET and HEAD must be presentation methods")
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if isNextPresentationMethod(method) {
			t.Fatalf("mutation method %s must stay on Go", method)
		}
	}
}

func TestWebFrontendMiddlewareKeepsProtocolsAPIsUnknownRoutesAndMutationsOnGo(t *testing.T) {
	server := &Server{WebFrontend: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request unexpectedly reached Next presentation")
	})}
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/dashboard/not-migrated"},
		{http.MethodGet, "/api/v1/account"},
		{http.MethodGet, "/.well-known/oauth-authorization-server"},
		{http.MethodGet, "/mcp"},
		{http.MethodGet, "/client"},
		{http.MethodPost, "/dashboard/devices/device/revoke"},
		{http.MethodPost, "/dashboard/workspaces/device/workspace/remove"},
		{http.MethodPost, "/login"},
		{http.MethodPost, "/signup"},
		{http.MethodPost, "/authorize"},
		{http.MethodPost, "/pair/approve"},
		{http.MethodPost, "/logout"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
			request := httptest.NewRequest(test.method, test.path, nil)
			server.webFrontendMiddleware(next).ServeHTTP(httptest.NewRecorder(), request)
			if !called {
				t.Fatal("expected Go handler to own request")
			}
		})
	}
}

func TestWebFrontendMiddlewareProxiesPublicNextPages(t *testing.T) {
	for _, path := range []string{"/", "/privacy", "/terms", "/support", "/security", "/forgot-password", "/reset-password", "/healthz", "/_next/static/app.js"} {
		t.Run(path, func(t *testing.T) {
			calledNext := false
			calledGo := false
			server := &Server{WebFrontend: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calledNext = true })}
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calledGo = true })
			server.webFrontendMiddleware(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
			if !calledNext || calledGo {
				t.Fatalf("expected public route %q to use Next only", path)
			}
		})
	}
}

func TestWebFrontendProxyIsOptionalAndValidatesConfiguredOrigin(t *testing.T) {
	t.Setenv("CODELOCAL_WEB_ORIGIN", "")
	proxy, err := newWebFrontendProxyFromEnv()
	if err != nil || proxy != nil {
		t.Fatalf("unconfigured presentation returned proxy=%v err=%v", proxy != nil, err)
	}

	for _, origin := range []string{"ftp://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?query=1"} {
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
