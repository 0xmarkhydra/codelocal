package cloudserver

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func newWebFrontendProxyFromEnv() (http.Handler, error) {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_WEB_ORIGIN"))
	if raw == "" {
		return nil, nil
	}
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil || (target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.Fragment != "" {
		return nil, errors.New("CODELOCAL_WEB_ORIGIN must be an http(s) origin without path, query, userinfo or fragment")
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	baseDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		baseDirector(request)
		// Go authenticates dashboard requests before this hop. The presentation
		// service does not need browser session secrets or client-IP signals.
		request.Header.Del("Authorization")
		request.Header.Del("Cookie")
		request.Header.Del("X-Real-IP")
		request.Header["X-Forwarded-For"] = nil
		request.Header.Del("X-Railway-Edge")
		request.Header.Del("X-Railway-Request-Id")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("next web frontend unavailable", "path", r.URL.Path, "error", err)
		http.Error(w, "CodeLocal web frontend is temporarily unavailable", http.StatusBadGateway)
	}
	return proxy, nil
}

func isNextPublicAssetPath(path string) bool {
	return strings.HasPrefix(path, "/_next/") || path == "/favicon.ico" || path == "/codelocal-icon.png"
}

var nextDashboardPaths = map[string]struct{}{
	"/dashboard":             {},
	"/dashboard/connect":     {},
	"/dashboard/workspaces":  {},
	"/dashboard/knowledge":   {},
	"/dashboard/code-graph":  {},
	"/dashboard/devices":     {},
	"/dashboard/usage":       {},
	"/dashboard/invite":      {},
	"/dashboard/leaderboard": {},
	"/dashboard/security":    {},
	"/dashboard/settings":    {},
	"/dashboard/account":     {},
	"/dashboard/skills":      {},
	"/dashboard/admin":       {},
}

var nextPublicPagePaths = map[string]struct{}{
	"/":                {},
	"/login":           {},
	"/register":        {},
	"/signup":          {},
	"/signup/verify":   {},
	"/forgot-password": {},
	"/reset-password":  {},
	"/privacy":         {},
	"/terms":           {},
	"/support":         {},
	"/security":        {},
	"/healthz":         {},
}

var nextFreshSecurityPaths = map[string]struct{}{
	"/authorize":    {},
	"/pair/approve": {},
}

// canonicalNextPresentationPath accepts the common single trailing slash
// variant without widening the presentation surface to arbitrary nested paths.
// Keeping normalization centralized is also important for the admin guard: the
// routing decision and authorization decision must always inspect the same path.
func canonicalNextPresentationPath(path string) string {
	if len(path) > 1 && strings.HasSuffix(path, "/") && !strings.HasSuffix(path, "//") {
		return strings.TrimSuffix(path, "/")
	}
	return path
}

func isNextDashboardPath(path string) bool {
	_, ok := nextDashboardPaths[canonicalNextPresentationPath(path)]
	return ok
}

func isNextPublicPagePath(path string) bool {
	_, ok := nextPublicPagePaths[canonicalNextPresentationPath(path)]
	return ok
}

func isNextFreshSecurityPath(path string) bool {
	_, ok := nextFreshSecurityPaths[canonicalNextPresentationPath(path)]
	return ok
}

func isNextPresentationMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func (s *Server) webFrontendMiddleware(next http.Handler) http.Handler {
	if s.WebFrontend == nil {
		return next
	}
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := s.WebAuth.Identity(r)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if identity == nil {
			nextPath := webutil.SafeNext(r.URL.RequestURI())
			http.Redirect(w, r, "/login?next="+url.QueryEscape(nextPath), http.StatusFound)
			return
		}
		if canonicalNextPresentationPath(r.URL.Path) == "/dashboard/admin" && !cloud.IsAdminEmail(identity.User.Email) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		s.WebFrontend.ServeHTTP(w, r)
	})
	protectedFresh := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := s.WebAuth.Identity(r)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if identity == nil {
			nextPath := webutil.SafeNext(r.URL.RequestURI())
			http.Redirect(w, r, "/login?next="+url.QueryEscape(nextPath), http.StatusFound)
			return
		}
		if !s.WebAuth.RequireFreshSecurityContext(w, r, identity) {
			return
		}
		s.WebFrontend.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isNextPresentationMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case isNextDashboardPath(r.URL.Path):
			protected.ServeHTTP(w, r)
		case isNextFreshSecurityPath(r.URL.Path):
			protectedFresh.ServeHTTP(w, r)
		case canonicalNextPresentationPath(r.URL.Path) == "/login" || canonicalNextPresentationPath(r.URL.Path) == "/signup":
			identity, err := s.WebAuth.Identity(r)
			if err != nil {
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}
			if identity != nil {
				http.Redirect(w, r, webutil.SafeNext(r.URL.Query().Get("next")), http.StatusFound)
				return
			}
			s.WebFrontend.ServeHTTP(w, r)
		case isNextPublicPagePath(r.URL.Path) || isNextPublicAssetPath(r.URL.Path):
			s.WebFrontend.ServeHTTP(w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}
