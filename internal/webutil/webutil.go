package webutil

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func DecodeJSON(r *http.Request, limit int64, dst any) error {
	if limit <= 0 {
		limit = 1 << 20
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	// Workspace sync and OAuth dynamic client registration are compatibility
	// boundaries. Older CodeLocal runtimes send local-only workspace metadata,
	// while RFC 7591/OAuth clients such as ChatGPT may send additional client
	// metadata fields (for example grant_types, response_types or
	// token_endpoint_auth_method). Ignore unknown fields on those endpoints so
	// the Go server matches the previous Express behavior, while keeping strict
	// JSON validation everywhere else.
	if r.URL.Path != "/api/client/workspaces/sync" && r.URL.Path != "/register" {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func ClientIP(r *http.Request) string {
	trustProxy := os.Getenv("CODELOCAL_TRUST_PROXY") == "1" || os.Getenv("RAILWAY_ENVIRONMENT") != "" || os.Getenv("RAILWAY_ENVIRONMENT_ID") != "" || os.Getenv("RAILWAY_PROJECT_ID") != ""
	if trustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			if len(forwarded) > 128 {
				return forwarded[:128]
			}
			return forwarded
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if len(r.RemoteAddr) > 128 {
		return r.RemoteAddr[:128]
	}
	return r.RemoteAddr
}

type RateLimitOptions struct {
	Scope   string
	Limit   int
	Window  time.Duration
	Subject func(*http.Request) string
}

func RateLimit(store *cloud.Store, options RateLimitOptions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject := ClientIP(r)
		if options.Subject != nil {
			if value := options.Subject(r); value != "" {
				subject = value
			}
		}
		window := int(options.Window.Seconds())
		if window <= 0 {
			window = 60
		}
		allowed, count, retry, err := store.RateLimit(r.Context(), options.Scope, subject, options.Limit, window)
		if err != nil {
			JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "rate_limit_unavailable"})
			return
		}
		w.Header().Set("RateLimit-Limit", itoa(options.Limit))
		remaining := options.Limit - count
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("RateLimit-Remaining", itoa(remaining))
		if !allowed {
			w.Header().Set("Retry-After", itoa(retry))
			JSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retryAfterSeconds": retry})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [32]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func SafeNext(value string) string {
	if value == "" {
		return "/dashboard"
	}
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, `\`) {
		return "/dashboard"
	}
	return value
}
