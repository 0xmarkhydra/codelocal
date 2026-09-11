package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeRuntimeBootstrapGeneratesLocalCompatibleCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != runtimeBootstrapExchangePath {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body runtimeBootstrapExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Token != "one-time-token" || body.RuntimeSessionID != "session-1" || body.DeviceID != "cloud-device-1" || body.PublicKey == "" {
			t.Errorf("request binding = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runtimeBootstrapExchangeResponse{
			CredentialID:     "cred-1",
			CredentialSecret: "secret-1",
			DeviceID:         "cloud-device-1",
			DeviceName:       "CodeLocal Cloud Runtime",
			WorkspaceID:      "workspace-1",
			WorkspaceKey:     "user::device::workspace-1",
			RuntimeSessionID: "session-1",
		})
	}))
	defer server.Close()

	credential, workspaceID, workspaceKey, err := ExchangeRuntimeBootstrap(context.Background(), RuntimeBootstrapConfig{
		ServerURL:        server.URL + "/ignored/path",
		Token:            "one-time-token",
		RuntimeSessionID: "session-1",
		DeviceID:         "cloud-device-1",
		HTTPClient:       server.Client(),
	})
	if err != nil {
		t.Fatalf("ExchangeRuntimeBootstrap() error = %v", err)
	}
	if credential.CredentialID != "cred-1" || credential.CredentialSecret != "secret-1" || credential.DevicePublicKey == "" || credential.DevicePrivateKey == "" {
		t.Fatalf("credential = %#v", credential)
	}
	if credential.ServerURL != server.URL || workspaceID != "workspace-1" || workspaceKey != "user::device::workspace-1" {
		t.Fatalf("binding server=%q workspaceID=%q workspaceKey=%q", credential.ServerURL, workspaceID, workspaceKey)
	}
}

func TestExchangeRuntimeBootstrapRejectsBindingMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runtimeBootstrapExchangeResponse{
			CredentialID:     "cred-1",
			CredentialSecret: "secret-1",
			DeviceID:         "different-device",
			WorkspaceID:      "workspace-1",
			RuntimeSessionID: "session-1",
		})
	}))
	defer server.Close()

	_, _, _, err := ExchangeRuntimeBootstrap(context.Background(), RuntimeBootstrapConfig{
		ServerURL:        server.URL,
		Token:            "one-time-token",
		RuntimeSessionID: "session-1",
		DeviceID:         "cloud-device-1",
		HTTPClient:       server.Client(),
	})
	if err == nil {
		t.Fatal("ExchangeRuntimeBootstrap() error = nil, want binding mismatch")
	}
}

func TestExchangeRuntimeBootstrapRequiresHTTPSOrHTTPServer(t *testing.T) {
	_, _, _, err := ExchangeRuntimeBootstrap(context.Background(), RuntimeBootstrapConfig{
		ServerURL:        "file:///tmp/server",
		Token:            "token",
		RuntimeSessionID: "session",
		DeviceID:         "device",
	})
	if err == nil {
		t.Fatal("ExchangeRuntimeBootstrap() error = nil, want invalid server URL")
	}
}
