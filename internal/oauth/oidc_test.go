package oauth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testOIDCConfig() oidcProviderConfig {
	return newOIDCProviderConfig(
		"https://codelocal.test", "signing-secret-with-at-least-thirty-two-bytes",
		"codelocal-penpot", "client-secret-with-at-least-24-bytes", "https://design.test/api/auth/oidc/callback",
	)
}

func TestOIDCSigningKeyIsStableAndPublishesNoSecret(t *testing.T) {
	first := testOIDCConfig()
	second := testOIDCConfig()
	if first.KeyID != second.KeyID || !first.PublicKey.Equal(second.PublicKey) {
		t.Fatal("OIDC signing identity must be stable for the configured CodeLocal signing secret")
	}
	if strings.Contains(base64.RawURLEncoding.EncodeToString(first.PublicKey), first.ClientSecret) {
		t.Fatal("OIDC public key exposed the client secret")
	}
}

func TestOIDCAccessTokenRoundTripAndTamperRejection(t *testing.T) {
	config := testOIDCConfig()
	now := time.Now()
	token, err := config.signToken(oidcTokenClaims{
		Issuer: config.Issuer, Subject: "user-1", Audience: config.ClientID,
		ExpiresAt: now.Add(time.Minute).Unix(), IssuedAt: now.Unix(), NotBefore: now.Add(-time.Second).Unix(),
		JWTID: "token-1", TokenUse: "access", ClientID: config.ClientID, Email: "user@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := config.verifyAccessToken(token, now)
	if err != nil || claims.Subject != "user-1" || claims.Email != "user@example.com" {
		t.Fatalf("claims=%#v err=%v", claims, err)
	}
	parts := strings.Split(token, ".")
	parts[1] = "x" + parts[1][1:]
	if _, err := config.verifyAccessToken(strings.Join(parts, "."), now); err == nil {
		t.Fatal("tampered OIDC token verified")
	}
}

func TestOIDCDiscoveryAndJWKSMatch(t *testing.T) {
	server := testServer()
	server.OIDC = testOIDCConfig()
	mux := http.NewServeMux()
	server.registerOIDC(mux)

	discovery := httptest.NewRecorder()
	mux.ServeHTTP(discovery, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
	if discovery.Code != http.StatusOK {
		t.Fatalf("discovery status=%d body=%s", discovery.Code, discovery.Body.String())
	}
	var metadata map[string]any
	if err := json.Unmarshal(discovery.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["authorization_endpoint"] != "https://codelocal.test/oauth/authorize" || metadata["userinfo_endpoint"] != "https://codelocal.test/oauth/userinfo" {
		t.Fatalf("unexpected discovery metadata: %#v", metadata)
	}

	jwks := httptest.NewRecorder()
	mux.ServeHTTP(jwks, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if jwks.Code != http.StatusOK || !strings.Contains(jwks.Body.String(), server.OIDC.KeyID) || strings.Contains(jwks.Body.String(), server.OIDC.ClientSecret) {
		t.Fatalf("unexpected JWKS response: %s", jwks.Body.String())
	}
}

func TestOIDCScopesRequireOpenID(t *testing.T) {
	if got, err := oidcScopes("email openid profile"); err != nil || got != "openid profile email" {
		t.Fatalf("scope=%q err=%v", got, err)
	}
	for _, raw := range []string{"profile email", "openid admin"} {
		if _, err := oidcScopes(raw); err == nil {
			t.Fatalf("invalid scopes accepted: %q", raw)
		}
	}
}

func TestOIDCTokenEndpointRejectsNonFormCredentials(t *testing.T) {
	server := testServer()
	server.OIDC = testOIDCConfig()
	request := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(`{"client_secret":"client-secret-with-at-least-24-bytes"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.oidcToken(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestOIDCClientAuthenticationRejectsMixedMethods(t *testing.T) {
	config := testOIDCConfig()
	server := testServer()
	server.OIDC = config
	request := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader("client_id="+url.QueryEscape(config.ClientID)+"&client_secret="+url.QueryEscape(config.ClientSecret)))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(config.ClientID, config.ClientSecret)
	if err := request.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if server.oidcClientAuthenticated(request) {
		t.Fatal("mixed OIDC client authentication methods were accepted")
	}
}
