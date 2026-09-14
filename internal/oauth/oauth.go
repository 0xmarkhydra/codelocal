package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

const Scope = "mcp:tools offline_access"

type tokenPayload struct {
	Type            string `json:"typ"`
	Subject         string `json:"sub"`
	ClientID        string `json:"client_id"`
	Resource        string `json:"resource"`
	Scope           string `json:"scope"`
	IssuedAt        int64  `json:"iat"`
	Expires         int64  `json:"exp"`
	JTI             string `json:"jti"`
	FamilyID        string `json:"fid,omitempty"`
	SecurityVersion int64  `json:"sv,omitempty"`
}

type Claims struct {
	Subject  string
	ClientID string
	Resource string
	Scope    string
}

type contextKey string

const claimsKey contextKey = "codelocal-oauth-claims"

type Server struct {
	Store      *cloud.Store
	WebAuth    *webauth.Manager
	BaseURL    string
	Resource   string
	Secret     []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	CodeTTL    time.Duration
	OIDC       oidcProviderConfig
}

func New(store *cloud.Store, auth *webauth.Manager, baseURL, secret string) (*Server, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" || secret == "" {
		return nil, errors.New("missing PUBLIC_BASE_URL or MCP_AUTH_SECRET")
	}
	server := &Server{Store: store, WebAuth: auth, BaseURL: baseURL, Resource: baseURL + "/mcp", Secret: []byte(secret), AccessTTL: time.Hour, RefreshTTL: 30 * 24 * time.Hour, CodeTTL: 5 * time.Minute}
	server.OIDC = oidcProviderConfigFromEnv(baseURL, secret)
	return server, nil
}

func randomURL(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
func (s *Server) sign(payload tokenPayload) string {
	raw, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Server) verify(token, expectedType string) (tokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return tokenPayload{}, errors.New("malformed token")
	}
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(actual) != len(expected) || subtle.ConstantTimeCompare(actual, expected) != 1 {
		return tokenPayload{}, errors.New("invalid token signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenPayload{}, err
	}
	var payload tokenPayload
	if json.Unmarshal(raw, &payload) != nil {
		return tokenPayload{}, errors.New("invalid token")
	}
	now := time.Now().Unix()
	if payload.Type != expectedType || payload.Expires <= now || payload.Resource != s.Resource || payload.Subject == "" {
		return tokenPayload{}, errors.New("expired or invalid token")
	}
	return payload, nil
}
func parseScope(value string) string {
	if value == "" {
		value = Scope
	}
	set := map[string]struct{}{}
	for _, part := range strings.Fields(value) {
		set[part] = struct{}{}
	}
	set["mcp:tools"] = struct{}{}
	set["offline_access"] = struct{}{}
	ordered := []string{}
	for _, part := range strings.Fields(value) {
		if _, ok := set[part]; ok {
			ordered = append(ordered, part)
			delete(set, part)
		}
	}
	for _, part := range []string{"mcp:tools", "offline_access"} {
		if _, ok := set[part]; ok {
			ordered = append(ordered, part)
			delete(set, part)
		}
	}
	return strings.Join(ordered, " ")
}
func (s *Server) tokenPair(userID, clientID, resource, scope, familyID string, securityVersion int64, refreshJTI string, issuedAt int64) map[string]any {
	scope = parseScope(scope)
	accessJTI := s.deriveTokenID("access-jti", familyID, refreshJTI)
	access := s.sign(tokenPayload{Type: "access", Subject: userID, ClientID: clientID, Resource: resource, Scope: scope, IssuedAt: issuedAt, Expires: issuedAt + int64(s.AccessTTL.Seconds()), JTI: accessJTI, FamilyID: familyID, SecurityVersion: securityVersion})
	refresh := s.sign(tokenPayload{Type: "refresh", Subject: userID, ClientID: clientID, Resource: resource, Scope: scope, IssuedAt: issuedAt, Expires: issuedAt + int64(s.RefreshTTL.Seconds()), JTI: refreshJTI, FamilyID: familyID, SecurityVersion: securityVersion})
	return map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": int64(s.AccessTTL.Seconds()), "scope": scope}
}

func validRedirect(value string) bool {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "https" {
		return u.Hostname() != ""
	}
	if scheme == "http" {
		host := strings.ToLower(u.Hostname())
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}

	// RFC 8252 permits public/native OAuth clients to use private-use URI
	// schemes. ChatGPT MCP registration may provide one of these callbacks,
	// while PKCE and exact redirect-URI binding still protect the auth code.
	switch scheme {
	case "javascript", "data", "file", "vbscript":
		return false
	}
	return u.Host != "" || u.Path != "" || u.Opaque != ""
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

type authorizeContext struct {
	ClientID      string `json:"clientId"`
	ClientName    string `json:"clientName"`
	RedirectURI   string `json:"redirectUri"`
	ResponseType  string `json:"responseType"`
	CodeChallenge string `json:"codeChallenge"`
	Resource      string `json:"resource"`
	Scope         string `json:"scope"`
	State         string `json:"state"`
	Email         string `json:"email"`
	CSRF          string `json:"csrf"`
}

func (s *Server) authorizeContextForRequest(r *http.Request) (*authorizeContext, *webauth.Identity, error) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	challenge := q.Get("code_challenge")
	method := q.Get("code_challenge_method")
	resource := q.Get("resource")
	scope := parseScope(q.Get("scope"))
	state := q.Get("state")
	client, _ := s.Store.OAuthClient(r.Context(), clientID)
	if client == nil || !contains(client.RedirectURIs, redirectURI) || responseType != "code" || challenge == "" || method != "S256" || resource != s.Resource {
		return nil, nil, errors.New("invalid OAuth authorization request")
	}
	identity, err := s.WebAuth.Identity(r)
	if err != nil {
		return nil, nil, err
	}
	if identity == nil {
		return nil, nil, nil
	}
	name := client.ClientName
	if name == "" {
		name = "MCP client"
	}
	return &authorizeContext{
		ClientID: clientID, ClientName: name, RedirectURI: redirectURI, ResponseType: responseType,
		CodeChallenge: challenge, Resource: resource, Scope: scope, State: state,
		Email: identity.User.Email, CSRF: identity.CSRF,
	}, identity, nil
}

func authorizeRetryURL(r *http.Request, errorMessage string) string {
	values := url.Values{
		"client_id":             {r.FormValue("client_id")},
		"redirect_uri":          {r.FormValue("redirect_uri")},
		"response_type":         {"code"},
		"code_challenge":        {r.FormValue("code_challenge")},
		"code_challenge_method": {"S256"},
		"resource":              {r.FormValue("resource")},
		"scope":                 {r.FormValue("scope")},
		"state":                 {r.FormValue("state")},
	}
	if errorMessage != "" {
		values.Set("error", errorMessage)
	}
	return "/authorize?" + values.Encode()
}

func (s *Server) Register(mux *http.ServeMux) {
	s.registerOIDC(mux)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		webutil.JSON(w, 200, map[string]any{"resource": s.Resource, "authorization_servers": []string{s.BaseURL}, "scopes_supported": []string{"mcp:tools", "offline_access"}, "bearer_methods_supported": []string{"header"}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		webutil.JSON(w, 200, map[string]any{"issuer": s.BaseURL, "authorization_endpoint": s.BaseURL + "/authorize", "token_endpoint": s.BaseURL + "/token", "registration_endpoint": s.BaseURL + "/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"mcp:tools", "offline_access"}})
	})
	register := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			RedirectURIs json.RawMessage `json:"redirect_uris"`
			RedirectURI  string          `json:"redirect_uri"`
			ClientName   string          `json:"client_name"`
		}
		if err := webutil.DecodeJSON(r, 64<<10, &input); err != nil {
			webutil.JSON(w, 400, map[string]any{"error": "invalid_client_metadata", "error_description": "Invalid JSON client metadata."})
			return
		}
		redirectURIs := []string{}
		if len(input.RedirectURIs) > 0 && string(input.RedirectURIs) != "null" {
			if err := json.Unmarshal(input.RedirectURIs, &redirectURIs); err != nil {
				var single string
				if err := json.Unmarshal(input.RedirectURIs, &single); err == nil && strings.TrimSpace(single) != "" {
					redirectURIs = []string{single}
				} else {
					webutil.JSON(w, 400, map[string]any{"error": "invalid_redirect_uri", "error_description": "redirect_uris must be a URI or an array of URIs."})
					return
				}
			}
		}
		if len(redirectURIs) == 0 && strings.TrimSpace(input.RedirectURI) != "" {
			redirectURIs = []string{input.RedirectURI}
		}
		if len(redirectURIs) == 0 {
			webutil.JSON(w, 400, map[string]any{"error": "invalid_redirect_uri", "error_description": "At least one redirect URI is required."})
			return
		}
		for i, uri := range redirectURIs {
			if !validRedirect(uri) {
				webutil.JSON(w, 400, map[string]any{"error": "invalid_redirect_uri", "error_description": fmt.Sprintf("Redirect URI at index %d is not an absolute OAuth callback URI.", i)})
				return
			}
		}
		client, err := s.Store.CreateOAuthClient(r.Context(), cloud.OAuthClient{ClientID: "codelocal_" + randomURL(24), RedirectURIs: redirectURIs, ClientName: truncate(input.ClientName, 160)})
		if err != nil {
			webutil.JSON(w, 500, map[string]any{"error": "server_error"})
			return
		}
		webutil.JSON(w, 201, map[string]any{"client_id": client.ClientID, "client_name": func() string {
			if client.ClientName != "" {
				return client.ClientName
			}
			return "MCP client"
		}(), "redirect_uris": client.RedirectURIs, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none"})
	})
	mux.Handle("POST /register", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "oauth-register-ip", Limit: 30, Window: time.Minute}, register))
	mux.HandleFunc("GET /api/v1/oauth/authorize-context", func(w http.ResponseWriter, r *http.Request) {
		ctx, identity, err := s.authorizeContextForRequest(r)
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_authorization_request"})
			return
		}
		if identity == nil {
			webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if identity.RequiresReauthentication() {
			webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "reauthentication_required"})
			return
		}
		webutil.JSON(w, http.StatusOK, ctx)
	})
	mux.HandleFunc("POST /authorize", func(w http.ResponseWriter, r *http.Request) {
		retryURL := authorizeRetryURL(r, "")
		identity, _ := s.WebAuth.Identity(r)
		if webauth.LocalOwnerMode() && identity != nil {
			// Auto-approve the registered MCP client for the single local owner.
			s.completeLocalOwnerAuthorize(w, r, identity, retryURL)
			return
		}
		if identity == nil {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(retryURL), http.StatusSeeOther)
			return
		}
		if !s.WebAuth.RequireFreshSecurityContext(w, r, identity, retryURL) {
			return
		}
		if !s.WebAuth.VerifyCSRF(r) {
			http.Redirect(w, r, authorizeRetryURL(r, "Invalid security token. Restart the authorization flow."), http.StatusSeeOther)
			return
		}
		clientID := r.FormValue("client_id")
		redirectURI := r.FormValue("redirect_uri")
		challenge := r.FormValue("code_challenge")
		resource := r.FormValue("resource")
		scope := parseScope(r.FormValue("scope"))
		state := r.FormValue("state")
		client, _ := s.Store.OAuthClient(r.Context(), clientID)
		if client == nil || !contains(client.RedirectURIs, redirectURI) || challenge == "" || resource != s.Resource {
			http.Error(w, "Invalid OAuth authorization request.", 400)
			return
		}
		code := randomURL(32)
		if err := s.Store.PutOAuthCode(r.Context(), cloud.OAuthCode{Code: code, UserID: identity.User.ID, ClientID: clientID, RedirectURI: redirectURI, CodeChallenge: challenge, Resource: resource, Scope: scope, ExpiresAt: time.Now().Add(s.CodeTTL).UnixMilli()}); err != nil {
			http.Error(w, "Unable to authorize", 500)
			return
		}
		s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "oauth.authorized", Detail: map[string]any{"clientId": clientID, "clientName": client.ClientName}})
		target, _ := url.Parse(redirectURI)
		params := target.Query()
		params.Set("code", code)
		if state != "" {
			params.Set("state", state)
		}
		target.RawQuery = params.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
	})
	tokenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		grant := r.Form.Get("grant_type")
		clientID := r.Form.Get("client_id")
		resource := r.Form.Get("resource")
		if grant == "authorization_code" {
			record, err := s.Store.ConsumeOAuthCode(r.Context(), r.Form.Get("code"))
			if err != nil || record == nil || record.ExpiresAt < time.Now().UnixMilli() || record.ClientID != clientID || record.RedirectURI != r.Form.Get("redirect_uri") || record.Resource != resource {
				webutil.JSON(w, 400, map[string]any{"error": "invalid_grant"})
				return
			}
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			challenge := base64.RawURLEncoding.EncodeToString(sum[:])
			if r.Form.Get("code_verifier") == "" || challenge != record.CodeChallenge {
				webutil.JSON(w, 400, map[string]any{"error": "invalid_grant"})
				return
			}
			tokens, err := s.issueInitial(r.Context(), record.UserID, clientID, resource, record.Scope)
			if err != nil {
				webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
				return
			}
			webutil.JSON(w, http.StatusOK, tokens)
			return
		}
		if grant == "refresh_token" {
			payload, err := s.verify(r.Form.Get("refresh_token"), "refresh")
			if err != nil || payload.ClientID != clientID || payload.Resource != resource {
				webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
				return
			}
			tokens, err := s.rotateRefresh(r.Context(), payload)
			if errors.Is(err, ErrTokenStateUnavailable) {
				webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
				return
			}
			if err != nil {
				webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
				return
			}
			webutil.JSON(w, http.StatusOK, tokens)
			return
		}
		webutil.JSON(w, 400, map[string]any{"error": "unsupported_grant_type"})
	})
	tokenClientRate := webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "oauth-token-client", Limit: 60, Window: time.Minute, Subject: func(r *http.Request) string { _ = r.ParseForm(); return truncate(r.Form.Get("client_id"), 160) }}, tokenHandler)
	mux.Handle("POST /token", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "oauth-token-ip", Limit: 120, Window: time.Minute}, tokenClientRate))
}

func truncate(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) > n {
		return v[:n]
	}
	return v
}

func (s *Server) RequireMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Single-operator self-host gateways resolve the local owner directly.
		// Workspace authorization, path policy and action approvals are still
		// enforced per call by the local runtime.
		if webauth.LocalOwnerMode() {
			// The account layer is removed, so the local-owner identity must not
			// be handed to anonymous internet callers when the gateway is
			// exposed through a tunnel. Loopback callers are unaffected.
			if !webauth.IsLoopbackRequest(r) && !webauth.RemoteAccessGated() {
				webutil.JSON(w, http.StatusForbidden, map[string]any{"error": "remote_access_disabled"})
				return
			}
			owner, err := s.WebAuth.LocalOwnerIdentity(r.Context())
			if err != nil || owner == nil {
				webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "local_owner_unavailable"})
				return
			}
			claims := Claims{Subject: owner.User.ID, ClientID: "local-owner", Resource: s.Resource, Scope: "mcp:tools offline_access"}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			s.unauthorized(w)
			return
		}
		payload, err := s.verify(strings.TrimPrefix(header, "Bearer "), "access")
		if err != nil {
			s.unauthorized(w)
			return
		}
		if err := s.validateTokenSecurity(r.Context(), payload); errors.Is(err, ErrTokenStateUnavailable) {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
			return
		} else if err != nil {
			s.unauthorized(w)
			return
		}
		if !contains(strings.Fields(payload.Scope), "mcp:tools") {
			webutil.JSON(w, 403, map[string]any{"error": "insufficient_scope"})
			return
		}
		claims := Claims{Subject: payload.Subject, ClientID: payload.ClientID, Resource: payload.Resource, Scope: payload.Scope}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
	})
}
func (s *Server) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="codelocal", resource_metadata="%s/.well-known/oauth-protected-resource"`, s.BaseURL))
	webutil.JSON(w, 401, map[string]any{"error": "unauthorized"})
}
func ClaimsFrom(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(Claims)
	return claims, ok
}
