package cloudserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/cloudmcp"
	"github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) pluginOAuthMetadata(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.WebAuth.PublicBaseURL, "/")
	webutil.JSON(w, 200, map[string]any{"client_id": base + "/api/v1/plugins/oauth/client", "client_name": "CodeLocal", "redirect_uris": []string{base + "/api/v1/plugins/oauth/callback"}, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none"})
}
func (s *Server) pluginOAuthStart(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.pluginMutationIdentity(w, r, true)
	if !ok {
		return
	}
	plugin := r.PathValue("pluginID")
	entry, exists := plugins.FindBuiltin(plugin)
	if !exists || !manifestSupportsCloud(entry) || plugin == "penpot" {
		webutil.JSON(w, 400, map[string]string{"error": "plugin_oauth_unsupported"})
		return
	}
	installation, installed, err := s.Store.PluginInstallationByID(r.Context(), identity.User.ID, plugin)
	if err != nil || !installed || installation.State != cloud.PluginInstalled {
		webutil.JSON(w, 409, map[string]string{"error": "plugin_not_installed"})
		return
	}
	var input struct {
		Endpoint string `json:"endpoint"`
	}
	if webutil.DecodeJSON(r, 4096, &input) != nil {
		webutil.JSON(w, 400, map[string]string{"error": "invalid_endpoint"})
		return
	}
	base := strings.TrimRight(s.WebAuth.PublicBaseURL, "/")
	if !strings.HasPrefix(base, "https://") {
		webutil.JSON(w, 503, map[string]string{"error": "oauth_requires_public_https"})
		return
	}
	state := cloud.RandomHex(32)
	client, closeClient := cloudmcp.NewMetadataClient()
	defer closeClient()
	flow, err := cloudmcp.DiscoverOAuth(r.Context(), client, input.Endpoint, base+"/api/v1/plugins/oauth/callback", base+"/api/v1/plugins/oauth/client", state)
	if err != nil {
		webutil.JSON(w, 400, map[string]string{"error": "oauth_discovery_failed", "detail": err.Error()})
		return
	}
	if err = s.Store.SavePluginOAuthFlow(r.Context(), state, identity.User.ID, identity.SessionID, plugin, flow); err != nil {
		webutil.JSON(w, 503, map[string]string{"error": "oauth_state_unavailable"})
		return
	}
	webutil.JSON(w, 200, map[string]string{"authorizationUrl": flow.AuthorizationURL})
}
func (s *Server) pluginOAuthCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	for _, name := range []string{"state", "code", "iss", "error"} {
		if len(q[name]) > 1 {
			http.Error(w, "Invalid OAuth response", 400)
			return
		}
	}
	var flow cloudmcp.OAuthFlow
	plugin, err := s.Store.TakePluginOAuthFlow(r.Context(), q.Get("state"), identity.User.ID, identity.SessionID, &flow)
	if err != nil {
		http.Error(w, "OAuth request expired or already used. Return to Plugins and reconnect.", 400)
		return
	}
	if q.Get("error") != "" {
		http.Error(w, "Authorization was not completed. Return to Plugins.", 400)
		return
	}
	client, closeClient := cloudmcp.NewMetadataClient()
	defer closeClient()
	token, err := cloudmcp.ExchangeOAuth(r.Context(), client, flow, q.Get("code"), q.Get("iss"))
	if err != nil {
		http.Error(w, "OAuth authorization failed. Return to Plugins and reconnect.", 400)
		return
	}
	credential := cloud.PluginCloudCredential{Kind: "oauth", Token: token.AccessToken, RefreshToken: token.RefreshToken, TokenEndpoint: flow.TokenEndpoint, ClientID: flow.ClientID, Issuer: flow.Issuer}
	if token.ExpiresIn > 0 {
		credential.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).Unix()
	}
	if s.CloudMCP == nil {
		http.Error(w, "Cloud MCP unavailable", 503)
		return
	}
	tools, err := s.CloudMCP.Discover(r.Context(), identity.User.ID, cloudmcp.Config{Endpoint: flow.Endpoint, Bearer: token.AccessToken})
	if err != nil {
		http.Error(w, "Sign-in succeeded but MCP discovery failed. Return to Plugins.", 502)
		return
	}
	_, err = s.Store.SavePluginCloudConnection(r.Context(), cloud.PluginConnection{UserID: identity.User.ID, PluginID: plugin, ServerName: "plugin-" + plugin, Endpoint: flow.Endpoint, State: cloud.PluginConnectionReady, ToolCount: len(tools)}, credential)
	if err != nil {
		http.Error(w, "Could not save plugin connection", 503)
		return
	}
	http.Redirect(w, r, "/dashboard/plugins", http.StatusSeeOther)
}
