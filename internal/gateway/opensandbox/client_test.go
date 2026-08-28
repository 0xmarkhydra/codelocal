package opensandbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientNormalizesLifecycleBaseURL(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://sandbox.example.com/"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.baseURL != "https://sandbox.example.com/v1" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}

func TestCreateSandboxUsesOfficialLifecyclePathAndAuthHeader(t *testing.T) {
	var received CreateSandboxRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get(defaultAuthHeader); got != "secret-key" {
			t.Fatalf("auth header = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"sb-1","entrypoint":["codelocal"],"metadata":{"workspace":"ws-1"},"status":{"state":"Creating"},"createdAt":"2026-08-29T00:00:00Z","expiresAt":null}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "secret-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	result, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
		Image:          &ImageSpec{URI: "codelocal/runtime:latest"},
		Entrypoint:     []string{"codelocal"},
		ResourceLimits: map[string]string{"cpu": "2", "memory": "4Gi"},
		Metadata:       map[string]string{"workspace": "ws-1"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox() error = %v", err)
	}
	if result.ID != "sb-1" || result.Status.State != "Creating" {
		t.Fatalf("CreateSandbox() result = %#v", result)
	}
	if received.Image == nil || received.Image.URI != "codelocal/runtime:latest" {
		t.Fatalf("received image = %#v", received.Image)
	}
	if received.ResourceLimits["memory"] != "4Gi" {
		t.Fatalf("received limits = %#v", received.ResourceLimits)
	}
}

func TestListSandboxesEncodesMetadataLikeOfficialSDK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query()["state"]; len(got) != 2 || got[0] != "Running" || got[1] != "Paused" {
			t.Fatalf("state query = %#v", got)
		}
		metadata := r.URL.Query().Get("metadata")
		if !strings.Contains(metadata, "tenant=t-1") || !strings.Contains(metadata, "workspace=ws-1") {
			t.Fatalf("metadata query = %q", metadata)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"pagination":{"page":1,"pageSize":20,"totalItems":0,"totalPages":0,"hasNextPage":false}}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.ListSandboxes(context.Background(), ListOptions{
		States:   []string{"Running", "Paused"},
		Metadata: map[string]string{"tenant": "t-1", "workspace": "ws-1"},
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListSandboxes() error = %v", err)
	}
}

func TestLifecycleActionsAndRenewExpiration(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/renew-expiration") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"expiresAt":"2026-08-29T02:00:00Z"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx := context.Background()
	if err := client.PauseSandbox(ctx, "sb/1"); err != nil {
		t.Fatal(err)
	}
	if err := client.ResumeSandbox(ctx, "sb/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RenewExpiration(ctx, "sb/1", time.Date(2026, 8, 29, 2, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteSandbox(ctx, "sb/1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /v1/sandboxes/sb%2F1/pause",
		"POST /v1/sandboxes/sb%2F1/resume",
		"POST /v1/sandboxes/sb%2F1/renew-expiration",
		"DELETE /v1/sandboxes/sb%2F1",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call[%d] = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestIsUnavailableSeparatesTransientAndHardFailures(t *testing.T) {
	if !IsUnavailable(&APIError{StatusCode: http.StatusServiceUnavailable}) {
		t.Fatal("503 should be unavailable")
	}
	if IsUnavailable(&APIError{StatusCode: http.StatusUnauthorized}) {
		t.Fatal("401 must not be treated as provider-unavailable fallback")
	}
	if !IsUnavailable(context.DeadlineExceeded) {
		t.Fatal("deadline should be unavailable")
	}
	if IsUnavailable(errors.New("workspace conflict")) {
		t.Fatal("hard error must not be unavailable")
	}
}

func TestCreateSandboxRequiresResourceLimits(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://sandbox.example.com"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{}); err == nil {
		t.Fatal("CreateSandbox() error = nil, want resource limit validation")
	}
}
