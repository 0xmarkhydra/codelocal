package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerPluginRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/plugins", s.pluginsResourceAPI)
	install := dashboardJSONCSRF(http.HandlerFunc(s.pluginInstallAPI))
	uninstall := dashboardJSONCSRF(http.HandlerFunc(s.pluginUninstallAPI))
	connect := dashboardJSONCSRF(http.HandlerFunc(s.pluginConnectAPI))
	disconnect := dashboardJSONCSRF(http.HandlerFunc(s.pluginDisconnectAPI))
	mux.Handle("POST /api/v1/plugins/{pluginID}/install", webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope: "plugin-install-ip", Limit: 30, Window: 10 * time.Minute,
	}, install))
	mux.Handle("DELETE /api/v1/plugins/{pluginID}/install", webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope: "plugin-uninstall-ip", Limit: 60, Window: 10 * time.Minute,
	}, uninstall))
	mux.Handle("POST /api/v1/plugins/{pluginID}/connections", webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope: "plugin-connect-ip", Limit: 30, Window: 10 * time.Minute,
	}, connect))
	mux.Handle("DELETE /api/v1/plugins/{pluginID}/connections/{deviceID}", webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope: "plugin-disconnect-ip", Limit: 60, Window: 10 * time.Minute,
	}, disconnect))
}
