package cloudserver

import "net/http"

// runtimeTransportMiddleware keeps runtime-node transport endpoints in the Go
// control plane even when browser presentation is proxied to Next.js. It is
// intentionally exact-path and exact-method so no generic API prefix is
// intercepted here.
func (s *Server) runtimeTransportMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/client/runtime/ws":
			s.runtimeRealtime(w, r)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/client/runtime/bootstrap/exchange":
			s.runtimeBootstrapExchange(w, r)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
