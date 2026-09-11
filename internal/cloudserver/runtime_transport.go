package cloudserver

import "net/http"

const (
	runtimeTransportRealtime  = "realtime"
	runtimeTransportBootstrap = "bootstrap"
)

func runtimeTransportRoute(method, path string) string {
	switch {
	case method == http.MethodGet && path == "/api/client/runtime/ws":
		return runtimeTransportRealtime
	case method == http.MethodPost && path == "/api/client/runtime/bootstrap/exchange":
		return runtimeTransportBootstrap
	default:
		return ""
	}
}

// runtimeTransportMiddleware keeps runtime-node transport endpoints in the Go
// control plane even when browser presentation is proxied to Next.js. It is
// intentionally exact-path and exact-method so no generic API prefix is
// intercepted here.
func (s *Server) runtimeTransportMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch runtimeTransportRoute(r.Method, r.URL.Path) {
		case runtimeTransportRealtime:
			s.runtimeRealtime(w, r)
			return
		case runtimeTransportBootstrap:
			s.runtimeBootstrapExchange(w, r)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
