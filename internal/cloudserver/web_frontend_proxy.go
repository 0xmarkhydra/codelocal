package cloudserver

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

func webFrontendCutoverEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_WEB_CUTOVER"))) {
	case "1", "true", "on", "enabled":
		return true
	default:
		return false
	}
}

func newWebFrontendProxyFromEnv() (http.Handler, error) {
	if !webFrontendCutoverEnabled() {
		return nil, nil
	}
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_WEB_ORIGIN"))
	if raw == "" {
		return nil, errors.New("CODELOCAL_WEB_ORIGIN is required when CODELOCAL_WEB_CUTOVER is enabled")
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

func isNextDashboardPath(path string) bool {
	if path == "/dashboard/admin" || strings.HasPrefix(path, "/dashboard/admin/") {
		return false
	}
	return path == "/dashboard" || strings.HasPrefix(path, "/dashboard/")
}

func (s *Server) webFrontendMiddleware(next http.Handler) http.Handler {
	if s.WebFrontend == nil {
		return next
	}
	protected := s.WebAuth.Require(s.WebFrontend)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isNextDashboardPath(r.URL.Path):
			protected.ServeHTTP(w, r)
		case r.URL.Path == "/" || r.URL.Path == "/healthz" || isNextPublicAssetPath(r.URL.Path):
			s.WebFrontend.ServeHTTP(w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}
