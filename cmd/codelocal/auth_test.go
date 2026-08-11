package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/identity"
)

func TestLoginArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		server  string
		force   bool
		wantErr bool
	}{
		{name: "empty", args: nil},
		{name: "gateway", args: []string{"https://example.test"}, server: "https://example.test"},
		{name: "force", args: []string{"--force"}, force: true},
		{name: "force gateway", args: []string{"--force", "https://example.test"}, server: "https://example.test", force: true},
		{name: "gateway force", args: []string{"https://example.test", "-f"}, server: "https://example.test", force: true},
		{name: "unknown option", args: []string{"--bad"}, wantErr: true},
		{name: "too many gateways", args: []string{"https://a.test", "https://b.test"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, force, err := loginArgs(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
			if server != test.server || force != test.force {
				t.Fatalf("got server=%q force=%v, want server=%q force=%v", server, force, test.server, test.force)
			}
		})
	}
}

func TestOptionalGatewayArgRejectsUnexpectedArguments(t *testing.T) {
	if _, err := optionalGatewayArg("logout", []string{"https://a.test", "https://b.test"}); err == nil {
		t.Fatal("expected too-many-arguments error")
	}
	if _, err := optionalGatewayArg("logout", []string{"--force"}); err == nil {
		t.Fatal("expected unknown-option error")
	}
}

func TestRevokeRemoteCredentialRetriesTransientFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	credential := identity.Credential{CredentialID: "cld_test", CredentialSecret: "secret", ServerURL: server.URL}
	revoked, err := revokeRemoteCredential(context.Background(), credential, "")
	if err != nil || !revoked {
		t.Fatalf("expected successful revoke after retries, revoked=%v err=%v", revoked, err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d, want 3", attempts)
	}
}

func TestRevokeRemoteCredentialDoesNotRetryPermanentFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	credential := identity.Credential{CredentialID: "cld_test", CredentialSecret: "secret", ServerURL: server.URL}
	revoked, err := revokeRemoteCredential(context.Background(), credential, "")
	if err == nil || revoked {
		t.Fatalf("expected permanent revoke failure, revoked=%v err=%v", revoked, err)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d, want 1", attempts)
	}
}

func TestValidateCredentialStatusClassification(t *testing.T) {
	credential := identity.Credential{CredentialID: "cld_test", CredentialSecret: "secret"}
	tests := []struct {
		name      string
		status    int
		wantValid bool
		wantErr   bool
	}{
		{name: "valid", status: http.StatusOK, wantValid: true},
		{name: "revoked", status: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "missing", status: http.StatusNotFound},
		{name: "server failure is transient", status: http.StatusInternalServerError, wantErr: true},
		{name: "rate limited is transient", status: http.StatusTooManyRequests, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				if test.status == http.StatusOK {
					_, _ = w.Write([]byte(`{"email":"user@example.com"}`))
				}
			}))
			defer server.Close()

			valid, email, err := validateCredential(context.Background(), server.URL, credential)
			if valid != test.wantValid {
				t.Fatalf("valid=%v, want %v", valid, test.wantValid)
			}
			if valid && email != "user@example.com" {
				t.Fatalf("unexpected email: %q", email)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
		})
	}
}
