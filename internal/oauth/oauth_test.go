package oauth

import (
	"strings"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func testServer() *Server {
	return &Server{BaseURL: "https://example.test", Resource: "https://example.test/mcp", Secret: []byte("test-secret-with-enough-entropy"), AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour, CodeTTL: 5 * time.Minute}
}

func TestTokenSignVerify(t *testing.T) {
	s := testServer()
	now := time.Now().Unix()
	token := s.sign(tokenPayload{Type: "access", Subject: "user-1", ClientID: "client-1", Resource: s.Resource, Scope: Scope, IssuedAt: now, Expires: now + 60, JTI: "jti"})
	payload, err := s.verify(token, "access")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Subject != "user-1" || payload.ClientID != "client-1" {
		t.Fatalf("payload=%#v", payload)
	}
	parts := strings.Split(token, ".")
	parts[0] = "x" + parts[0][1:]
	if _, err := s.verify(strings.Join(parts, "."), "access"); err == nil {
		t.Fatal("tampered token verified")
	}
}

func TestTokenRejectsWrongTypeResourceAndExpiry(t *testing.T) {
	s := testServer()
	now := time.Now().Unix()
	refresh := s.sign(tokenPayload{Type: "refresh", Subject: "u", ClientID: "c", Resource: s.Resource, Scope: Scope, IssuedAt: now, Expires: now + 60, JTI: "j"})
	if _, err := s.verify(refresh, "access"); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
	wrongResource := s.sign(tokenPayload{Type: "access", Subject: "u", ClientID: "c", Resource: "https://other.test/mcp", Scope: Scope, IssuedAt: now, Expires: now + 60, JTI: "j"})
	if _, err := s.verify(wrongResource, "access"); err == nil {
		t.Fatal("wrong resource token verified")
	}
	expired := s.sign(tokenPayload{Type: "access", Subject: "u", ClientID: "c", Resource: s.Resource, Scope: Scope, IssuedAt: now - 120, Expires: now - 1, JTI: "j"})
	if _, err := s.verify(expired, "access"); err == nil {
		t.Fatal("expired token verified")
	}
}

func TestRedirectPolicy(t *testing.T) {
	for _, allowed := range []string{
		"https://chatgpt.com/oauth/callback",
		"http://localhost:1234/callback",
		"http://127.0.0.1/callback",
		"http://[::1]:8080/callback",
		"chatgpt://oauth/callback",
		"com.openai.chatgpt:/oauth/callback",
	} {
		if !validRedirect(allowed) {
			t.Fatalf("allowed redirect rejected: %s", allowed)
		}
	}
	for _, blocked := range []string{
		"http://example.com/callback",
		"javascript:alert(1)",
		"data:text/plain,hello",
		"file:///tmp/callback",
		"https://user:pass@example.com/callback",
		"https://example.com/callback#fragment",
		"/relative",
	} {
		if validRedirect(blocked) {
			t.Fatalf("unsafe redirect accepted: %s", blocked)
		}
	}
}

func TestParseScopeAlwaysIncludesRequiredScopes(t *testing.T) {
	scope := parseScope("custom mcp:tools")
	fields := strings.Fields(scope)
	for _, required := range []string{"custom", "mcp:tools", "offline_access"} {
		if !contains(fields, required) {
			t.Fatalf("scope %q missing %q", scope, required)
		}
	}
}

func TestTokenSecurityVersionAndLegacyCompatibility(t *testing.T) {
	state := cloud.UserSecurityState{Version: 3, PasswordChangedAt: 101_000}
	if err := validateTokenAgainstSecurityState(tokenPayload{SecurityVersion: 3}, state); err != nil {
		t.Fatalf("current security version rejected: %v", err)
	}
	if err := validateTokenAgainstSecurityState(tokenPayload{SecurityVersion: 2}, state); err != ErrTokenRevoked {
		t.Fatalf("stale security version error=%v want revoked", err)
	}
	if err := validateTokenAgainstSecurityState(tokenPayload{IssuedAt: 100}, state); err != ErrTokenRevoked {
		t.Fatalf("legacy token issued before password change error=%v want revoked", err)
	}
	if err := validateTokenAgainstSecurityState(tokenPayload{IssuedAt: 102}, state); err != nil {
		t.Fatalf("legacy token issued after password change rejected: %v", err)
	}
}

func TestOAuthFamiliesDoNotCollideForSameClient(t *testing.T) {
	first := oauthFamilyKey("user-1", "chatgpt", "family-a")
	second := oauthFamilyKey("user-1", "chatgpt", "family-b")
	if first == second {
		t.Fatal("independent OAuth authorizations must not share token-family state")
	}
	if strings.Contains(first, "user-1") || strings.Contains(first, "chatgpt") {
		t.Fatal("OAuth family Redis key must not expose raw user or client identifiers")
	}
}
