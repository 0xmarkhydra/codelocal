package oauth

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

// AutoAuthorizeLocalOwner issues an MCP authorization code for the single local
// owner without an interactive approval, reading the request from the query
// string (GET) or form (POST). Used only when CODELOCAL_LOCAL_OWNER is enabled
// on an operator-owned gateway.
func (s *Server) AutoAuthorizeLocalOwner(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	values := r.URL.Query()
	_ = r.ParseForm()
	for key, list := range r.Form {
		if len(list) > 0 {
			values.Set(key, list[0])
		}
	}
	s.issueLocalOwnerCode(w, r, identity, values.Get("client_id"), values.Get("redirect_uri"),
		values.Get("code_challenge"), values.Get("resource"), parseScope(values.Get("scope")), values.Get("state"))
}

// completeLocalOwnerAuthorize handles the POST form variant.
func (s *Server) completeLocalOwnerAuthorize(w http.ResponseWriter, r *http.Request, identity *webauth.Identity, retryURL string) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, authorizeRetryURL(r, "Invalid authorization request."), http.StatusSeeOther)
		return
	}
	s.issueLocalOwnerCode(w, r, identity, r.FormValue("client_id"), r.FormValue("redirect_uri"),
		r.FormValue("code_challenge"), r.FormValue("resource"), parseScope(r.FormValue("scope")), r.FormValue("state"))
}

func (s *Server) issueLocalOwnerCode(w http.ResponseWriter, r *http.Request, identity *webauth.Identity, clientID, redirectURI, challenge, resource, scope, state string) {

	client, _ := s.Store.OAuthClient(r.Context(), clientID)
	if client == nil || !contains(client.RedirectURIs, redirectURI) || challenge == "" || resource != s.Resource {
		http.Error(w, "Invalid OAuth authorization request.", http.StatusBadRequest)
		return
	}

	code := randomURL(32)
	if err := s.Store.PutOAuthCode(r.Context(), cloud.OAuthCode{
		Code: code, UserID: identity.User.ID, ClientID: clientID, RedirectURI: redirectURI,
		CodeChallenge: challenge, Resource: resource, Scope: scope,
		ExpiresAt: time.Now().Add(s.CodeTTL).UnixMilli(),
	}); err != nil {
		http.Error(w, "Unable to authorize", 500)
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "oauth.authorized", Detail: map[string]any{"clientId": clientID, "clientName": client.ClientName, "mode": "local-owner"}})

	target, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid redirect URI: %v", err), 500)
		return
	}
	params := target.Query()
	params.Set("code", code)
	if state != "" {
		params.Set("state", state)
	}
	target.RawQuery = params.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}
