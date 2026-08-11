package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/identity"
	"github.com/0xmarkhydra/codelocal/internal/workspace"
)

func TestTerminalToolContextOmitsMCPRequestIdentity(t *testing.T) {
	got := terminalToolContext(workspace.Workspace{WorkspaceName: "codex-mcp", WorkspaceID: "codex-mcp-e759036dfa"})
	if got != "codex-mcp · codex-mcp-e759036dfa" {
		t.Fatalf("unexpected context: %q", got)
	}
}

func TestTerminalTimestampFormat(t *testing.T) {
	got := terminalTimestamp(time.Date(2026, time.August, 31, 12, 15, 0, 0, time.Local))
	if got != "08/31/2026 - 12:15" {
		t.Fatalf("unexpected timestamp: %q", got)
	}
}

func TestTerminalToolDetailRedactsCommandSecrets(t *testing.T) {
	got := terminalToolDetail(map[string]any{"command": "GITHUB_TOKEN=super-secret npm test"})
	if got == "" || got == "  $ GITHUB_TOKEN=super-secret npm test" {
		t.Fatalf("command was not safely rendered: %q", got)
	}
}

func TestTerminalTraceColorHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if got := terminalTraceColor(terminalANSIGreen, "✓"); got != "✓" {
		t.Fatalf("NO_COLOR should disable ANSI styling: %q", got)
	}
}

func TestPostReturnsDeviceAuthorizationRevokedSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	runtime := New(Options{
		BaseURL: server.URL,
		Credential: identity.Credential{
			CredentialID:     "cld_test",
			CredentialSecret: "secret",
		},
	})
	err := runtime.post(context.Background(), "/api/client/runtime/poll", map[string]any{}, nil)
	if !errors.Is(err, ErrDeviceAuthorizationRevoked) {
		t.Fatalf("expected ErrDeviceAuthorizationRevoked, got %v", err)
	}
}
