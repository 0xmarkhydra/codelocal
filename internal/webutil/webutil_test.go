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
