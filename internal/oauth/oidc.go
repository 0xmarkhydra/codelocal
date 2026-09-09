package oauth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

const (
	defaultPenpotOIDCClientID    = "codelocal-penpot"
	defaultPenpotOIDCRedirectURI = "https://design.codelocal.cloud/api/auth/oidc/callback"
	oidcAuthorizationCodeTTL     = 5 * time.Minute
	defaultOIDCAccessTTL         = time.Hour
)

type oidcProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Issuer       string
	KeyID        string
	PrivateKey   ed25519.PrivateKey
	PublicKey    ed25519.PublicKey
	AccessTTL    time.Duration
}

func oidcProviderConfigFromEnv(baseURL, signingSecret string) oidcProviderConfig {
	clientID := strings.TrimSpace(os.Getenv("CODELOCAL_PENPOT_OIDC_CLIENT_ID"))
	if clientID == "" {
		clientID = defaultPenpotOIDCClientID
	}
	redirectURI := strings.TrimSpace(os.Getenv("CODELOCAL_PENPOT_OIDC_REDIRECT_URI"))
	if redirectURI == "" {
		redirectURI = defaultPenpotOIDCRedirectURI
	}
	return newOIDCProviderConfig(baseURL, signingSecret, clientID, os.Getenv("CODELOCAL_PENPOT_OIDC_CLIENT_SECRET"), redirectURI)
}

func newOIDCProviderConfig(baseURL, signingSecret, clientID, clientSecret, redirectURI string) oidcProviderConfig {
	issuer := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	seed := sha256.Sum256([]byte("codelocal-oidc-ed25519-v1\x00" + signingSecret))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	kidSum := sha256.Sum256(publicKey)
	return oidcProviderConfig{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		RedirectURI:  strings.TrimSpace(redirectURI),
		Issuer:       issuer,
		KeyID:        base64.RawURLEncoding.EncodeToString(kidSum[:12]),
		PrivateKey:   privateKey,
		PublicKey:    publicKey,
		AccessTTL:    defaultOIDCAccessTTL,
	}
}

func (c oidcProviderConfig) enabled() bool {
	redirect, err := url.Parse(c.RedirectURI)
	return c.Issuer != "" && c.ClientID != "" && len(c.ClientSecret) >= 24 && err == nil && redirect.Scheme == "https" && redirect.Hostname() != "" && len(c.PrivateKey) == ed25519.PrivateKeySize
}

type oidcTokenClaims struct {
	Issuer          string `json:"iss"`
	Subject         string `json:"sub"`
	Audience        string `json:"aud"`
	ExpiresAt       int64  `json:"exp"`
	IssuedAt        int64  `json:"iat"`
	NotBefore       int64  `json:"nbf"`
	AuthTime        int64  `json:"auth_time,omitempty"`
	JWTID           string `json:"jti"`
	TokenUse        string `json:"token_use"`
	ClientID        string `json:"client_id"`
	Scope           string `json:"scope,omitempty"`
	Nonce           string `json:"nonce,omitempty"`
	Name            string `json:"name,omitempty"`
	Email           string `json:"email,omitempty"`
	EmailVerified   bool   `json:"email_verified,omitempty"`
	SecurityVersion int64  `json:"sv,omitempty"`
}

func encodeJWTPart(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (c oidcProviderConfig) signToken(claims any) (string, error) {
	header, err := encodeJWTPart(map[string]string{"alg": "EdDSA", "kid": c.KeyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}
	signed := header + "." + payload
	signature := ed25519.Sign(c.PrivateKey, []byte(signed))
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c oidcProviderConfig) verifySignedToken(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed OIDC token")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerRaw, &header) != nil || header.Algorithm != "EdDSA" || header.KeyID != c.KeyID {
		return nil, errors.New("invalid OIDC token header")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(c.PublicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, errors.New("invalid OIDC token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid OIDC token payload")
	}
	return payload, nil
}

func (c oidcProviderConfig) verifyAccessToken(token string, now time.Time) (oidcTokenClaims, error) {
	payload, err := c.verifySignedToken(token)
	if err != nil {
		return oidcTokenClaims{}, err
	}
	var claims oidcTokenClaims
	if json.Unmarshal(payload, &claims) != nil {
		return oidcTokenClaims{}, errors.New("invalid OIDC token claims")
	}
	nowUnix := now.Unix()
	if claims.Issuer != c.Issuer || claims.Audience != c.ClientID || claims.ClientID != c.ClientID || claims.Subject == "" || claims.TokenUse != "access" || claims.ExpiresAt <= nowUnix || claims.NotBefore > nowUnix+30 || claims.IssuedAt > nowUnix+30 {
		return oidcTokenClaims{}, errors.New("expired or invalid OIDC token")
	}
	return claims, nil
}

func oidcScopes(raw string) (string, error) {
	seen := map[string]bool{}
	for _, scope := range strings.Fields(raw) {
		switch scope {
		case "openid", "profile", "email":
			seen[scope] = true
		default:
			return "", fmt.Errorf("unsupported OIDC scope %q", scope)
		}
	}
	if !seen["openid"] {
		return "", errors.New("openid scope is required")
	}
	ordered := []string{"openid"}
	for _, scope := range []string{"profile", "email"} {
		if seen[scope] {
			ordered = append(ordered, scope)
		}
	}
	return strings.Join(ordered, " "), nil
}

func oidcDisplayName(email string) string {
	local, _, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" {
		return strings.TrimSpace(email)
	}
	return local
}

func (s *Server) oidcAuthorizationError(w http.ResponseWriter, r *http.Request, code, description string) {
	q := r.URL.Query()
	if q.Get("client_id") == s.OIDC.ClientID && q.Get("redirect_uri") == s.OIDC.RedirectURI {
		target, _ := url.Parse(s.OIDC.RedirectURI)
		values := target.Query()
		values.Set("error", code)
		values.Set("error_description", description)
		if state := q.Get("state"); state != "" {
			values.Set("state", state)
		}
		target.RawQuery = values.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}
	webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": code, "error_description": description})
}

func (s *Server) oidcAuthorize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.OIDC.enabled() {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable", "error_description": "CodeLocal Penpot SSO is not configured."})
		return
	}
	q := r.URL.Query()
	if q.Get("client_id") != s.OIDC.ClientID || q.Get("redirect_uri") != s.OIDC.RedirectURI || q.Get("response_type") != "code" {
		s.oidcAuthorizationError(w, r, "invalid_request", "Invalid OIDC authorization request.")
		return
	}
	scope, err := oidcScopes(q.Get("scope"))
	if err != nil {
		s.oidcAuthorizationError(w, r, "invalid_scope", err.Error())
		return
	}
	challenge := strings.TrimSpace(q.Get("code_challenge"))
	if challenge != "" && (q.Get("code_challenge_method") != "S256" || len(challenge) > 128) {
		s.oidcAuthorizationError(w, r, "invalid_request", "Only PKCE S256 is supported.")
		return
	}
	nonce := strings.TrimSpace(q.Get("nonce"))
	if len(nonce) > 512 || len(q.Get("state")) > 2048 {
		s.oidcAuthorizationError(w, r, "invalid_request", "OIDC state is too large.")
		return
	}
	identity, err := s.WebAuth.Identity(r)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	if identity == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	if !s.WebAuth.RequireFreshSecurityContext(w, r, identity, r.URL.RequestURI()) {
		return
	}
	code := randomURL(32)
	now := time.Now()
	record := cloud.OIDCAuthorizationCode{
		UserID: identity.User.ID, ClientID: s.OIDC.ClientID, RedirectURI: s.OIDC.RedirectURI,
		Scope: scope, Nonce: nonce, CodeChallenge: challenge, AuthTime: now.Unix(), ExpiresAt: now.Add(oidcAuthorizationCodeTTL).UnixMilli(),
	}
	if err := s.Store.PutOIDCAuthorizationCode(r.Context(), code, record, oidcAuthorizationCodeTTL); err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "oidc.penpot_authorized", Detail: map[string]any{"clientId": s.OIDC.ClientID}})
	target, _ := url.Parse(s.OIDC.RedirectURI)
	values := target.Query()
	values.Set("code", code)
	if state := q.Get("state"); state != "" {
		values.Set("state", state)
	}
	target.RawQuery = values.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func secureStringEqual(expected, actual string) bool {
	a, b := []byte(expected), []byte(actual)
	return len(a) > 0 && len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func (s *Server) oidcClientAuthenticated(r *http.Request) bool {
	clientID, clientSecret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	if basicID, basicSecret, ok := r.BasicAuth(); ok {
		if clientID != "" || clientSecret != "" {
			return false
		}
		clientID, clientSecret = basicID, basicSecret
	}
	return secureStringEqual(s.OIDC.ClientID, clientID) && secureStringEqual(s.OIDC.ClientSecret, clientSecret)
}

func (s *Server) oidcToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !s.OIDC.enabled() {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/x-www-form-urlencoded" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request", "error_description": "Token requests must use form encoding."})
		return
	}
	if err := r.ParseForm(); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if !s.oidcClientAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="codelocal-penpot-oidc"`)
		webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	if r.Form.Get("grant_type") != "authorization_code" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	record, err := s.Store.ConsumeOIDCAuthorizationCode(r.Context(), r.Form.Get("code"))
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	if record == nil || record.ExpiresAt <= time.Now().UnixMilli() || record.ClientID != s.OIDC.ClientID || record.RedirectURI != r.Form.Get("redirect_uri") {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	if record.CodeChallenge != "" {
		verifier := r.Form.Get("code_verifier")
		digest := sha256.Sum256([]byte(verifier))
		if verifier == "" || !secureStringEqual(record.CodeChallenge, base64.RawURLEncoding.EncodeToString(digest[:])) {
			webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
	}
	user, err := s.Store.UserByID(r.Context(), record.UserID)
	if err != nil || user == nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	securityState, err := s.userSecurityState(r.Context(), user.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	now := time.Now()
	expires := now.Add(s.OIDC.AccessTTL)
	baseClaims := oidcTokenClaims{
		Issuer: s.OIDC.Issuer, Subject: user.ID, Audience: s.OIDC.ClientID,
		ExpiresAt: expires.Unix(), IssuedAt: now.Unix(), NotBefore: now.Add(-5 * time.Second).Unix(), AuthTime: record.AuthTime,
		ClientID: s.OIDC.ClientID, Name: oidcDisplayName(user.Email), Email: user.Email, EmailVerified: true,
		SecurityVersion: securityState.Version,
	}
	accessClaims := baseClaims
	accessClaims.JWTID, accessClaims.TokenUse, accessClaims.Scope = randomURL(18), "access", record.Scope
	idClaims := baseClaims
	idClaims.JWTID, idClaims.TokenUse, idClaims.Nonce = randomURL(18), "id", record.Nonce
	accessToken, accessErr := s.OIDC.signToken(accessClaims)
	idToken, idErr := s.OIDC.signToken(idClaims)
	if accessErr != nil || idErr != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken, "id_token": idToken, "token_type": "Bearer",
		"expires_in": int64(s.OIDC.AccessTTL.Seconds()), "scope": record.Scope,
	})
}

func (s *Server) oidcUserInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.OIDC.enabled() {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") {
		s.oidcUnauthorized(w)
		return
	}
	claims, err := s.OIDC.verifyAccessToken(strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), time.Now())
	if err != nil {
		s.oidcUnauthorized(w)
		return
	}
	if err := s.validateTokenSecurity(r.Context(), tokenPayload{Subject: claims.Subject, IssuedAt: claims.IssuedAt, SecurityVersion: claims.SecurityVersion}); err != nil {
		s.oidcUnauthorized(w)
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"sub": claims.Subject, "name": claims.Name, "email": claims.Email, "email_verified": claims.EmailVerified,
	})
}

func (s *Server) oidcUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="codelocal-penpot-oidc", error="invalid_token"`)
	webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
}

func (s *Server) registerOIDC(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		webutil.JSON(w, http.StatusOK, map[string]any{
			"issuer": s.OIDC.Issuer, "authorization_endpoint": s.OIDC.Issuer + "/oauth/authorize",
			"token_endpoint": s.OIDC.Issuer + "/oauth/token", "userinfo_endpoint": s.OIDC.Issuer + "/oauth/userinfo",
			"jwks_uri": s.OIDC.Issuer + "/.well-known/jwks.json", "response_types_supported": []string{"code"},
			"grant_types_supported": []string{"authorization_code"}, "subject_types_supported": []string{"public"},
			"id_token_signing_alg_values_supported": []string{"EdDSA"}, "scopes_supported": []string{"openid", "profile", "email"},
			"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "name", "email", "email_verified"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
			"code_challenge_methods_supported":      []string{"S256"},
		})
	})
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
		webutil.JSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
			"kty": "OKP", "crv": "Ed25519", "use": "sig", "alg": "EdDSA", "kid": s.OIDC.KeyID,
			"x": base64.RawURLEncoding.EncodeToString(s.OIDC.PublicKey),
		}}})
	})
	mux.Handle("GET /oauth/authorize", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "oidc-penpot-authorize-ip", Limit: 60, Window: time.Minute}, http.HandlerFunc(s.oidcAuthorize)))
	mux.Handle("POST /oauth/token", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "oidc-penpot-token-ip", Limit: 120, Window: time.Minute}, http.HandlerFunc(s.oidcToken)))
	mux.HandleFunc("GET /oauth/userinfo", s.oidcUserInfo)
	mux.HandleFunc("POST /oauth/userinfo", s.oidcUserInfo)
}
