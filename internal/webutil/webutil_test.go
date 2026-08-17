package webutil

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONAllowsLegacyWorkspaceSyncMetadata(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/client/workspaces/sync", strings.NewReader(`{"workspaces":[{"workspaceId":"demo","workspaceName":"Demo","grantedAt":123,"lastActivatedAt":456}]}`))
	var input struct {
		Workspaces []struct {
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
		} `json:"workspaces"`
	}
	if err := DecodeJSON(req, 1<<20, &input); err != nil {
		t.Fatalf("legacy workspace sync payload should be accepted: %v", err)
	}
	if len(input.Workspaces) != 1 || input.Workspaces[0].WorkspaceID != "demo" {
		t.Fatalf("unexpected decoded payload: %#v", input)
	}
}

func TestDecodeJSONAllowsOAuthDynamicClientMetadata(t *testing.T) {
	req := httptest.NewRequest("POST", "/register", strings.NewReader(`{"redirect_uris":["https://chatgpt.com/oauth/callback"],"client_name":"ChatGPT","grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`))
	var input struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	if err := DecodeJSON(req, 1<<20, &input); err != nil {
		t.Fatalf("OAuth DCR metadata extensions should be accepted: %v", err)
	}
	if len(input.RedirectURIs) != 1 || input.RedirectURIs[0] != "https://chatgpt.com/oauth/callback" {
		t.Fatalf("unexpected decoded OAuth payload: %#v", input)
	}
}

func TestDecodeJSONRemainsStrictElsewhere(t *testing.T) {
	req := httptest.NewRequest("POST", "/pair/claim", strings.NewReader(`{"pairingId":"p","code":"c","unexpected":true}`))
	var input struct {
		PairingID string `json:"pairingId"`
		Code      string `json:"code"`
	}
	if err := DecodeJSON(req, 1<<20, &input); err == nil {
		t.Fatal("unknown fields should remain rejected outside compatibility endpoints")
	}
}

func TestClientIPPrefersRailwayRealIP(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_ID", "test")
	req := httptest.NewRequest("GET", "https://example.test", nil)
	req.RemoteAddr = "10.0.0.7:4321"
	req.Header.Set("X-Real-IP", "203.0.113.42")
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.1")
	if got := ClientIP(req); got != "203.0.113.42" {
		t.Fatalf("ClientIP=%q want Railway X-Real-IP", got)
	}
}

func TestClientIPFallsBackSafely(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_ID", "test")
	req := httptest.NewRequest("GET", "https://example.test", nil)
	req.RemoteAddr = "10.0.0.7:4321"
	req.Header.Set("X-Real-IP", "not-an-ip")
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.1")
	if got := ClientIP(req); got != "198.51.100.9" {
		t.Fatalf("ClientIP=%q want validated X-Forwarded-For fallback", got)
	}
	req.Header.Set("X-Forwarded-For", "also-not-an-ip")
	if got := ClientIP(req); got != "10.0.0.7" {
		t.Fatalf("ClientIP=%q want RemoteAddr fallback", got)
	}
}
