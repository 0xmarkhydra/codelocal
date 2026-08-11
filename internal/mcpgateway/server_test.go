package mcpgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/clientupdate"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestUpdateNoticeShownOncePerWorkspaceReleaseAndSession(t *testing.T) {
	s := &Service{
		Release: clientupdate.Manifest{
			LatestVersion:  "1.5.0-beta.5",
			MinimumVersion: "1.5.0-beta.4",
			Channel:        "beta",
			UpdateCommand:  "npm i -g codelocal@beta",
			RestartCommand: "codelocal",
			Message:        "Update available.",
		},
		shownUpdates: map[string]map[string]struct{}{},
	}

	first := s.claimUpdate("user-1", "session-1", "device::workspace", "1.5.0-beta.4")
	if !strings.Contains(first, "[CODELOCAL_UPDATE_NOTICE]") {
		t.Fatalf("expected update notice, got %q", first)
	}
	if second := s.claimUpdate("user-1", "session-1", "device::workspace", "1.5.0-beta.4"); second != "" {
		t.Fatalf("same session/release should not repeat notice: %q", second)
	}
	if otherSession := s.claimUpdate("user-1", "session-2", "device::workspace", "1.5.0-beta.4"); otherSession == "" {
		t.Fatal("new MCP session should receive the notice")
	}
}

func TestUpdateNoticeIsScopedPerWorkspace(t *testing.T) {
	s := &Service{
		Release:      clientupdate.Manifest{LatestVersion: "2.0.0", Channel: "beta", UpdateCommand: "npm i -g codelocal@beta", RestartCommand: "codelocal", Message: "Update."},
		shownUpdates: map[string]map[string]struct{}{},
	}
	if s.claimUpdate("u", "s", "workspace-a", "1.0.0") == "" {
		t.Fatal("workspace-a should receive notice")
	}
	if s.claimUpdate("u", "s", "workspace-b", "1.0.0") == "" {
		t.Fatal("workspace-b should receive its own notice")
	}
}

func definitionNamed(t *testing.T, name string) toolDef {
	t.Helper()
	for _, def := range toolDefinitions() {
		if def.Name == name {
			return def
		}
	}
	t.Fatalf("tool definition not found: %s", name)
	return toolDef{}
}

func TestToolMetadataMakesCommonCallsUnderstandable(t *testing.T) {
	status := definitionNamed(t, "git_status")
	if got := toolDisplayTitle(status.Name, status.Title); got != "Check Git status" {
		t.Fatalf("unexpected git_status title: %q", got)
	}
	statusAnnotations := toolAnnotations(status)
	if !statusAnnotations.ReadOnlyHint || statusAnnotations.OpenWorldHint == nil || *statusAnnotations.OpenWorldHint {
		t.Fatalf("git_status annotations should describe a closed-world read-only action: %#v", statusAnnotations)
	}

	push := definitionNamed(t, "git_push")
	pushAnnotations := toolAnnotations(push)
	if pushAnnotations.ReadOnlyHint || pushAnnotations.OpenWorldHint == nil || !*pushAnnotations.OpenWorldHint {
		t.Fatalf("git_push annotations should describe an external side effect: %#v", pushAnnotations)
	}
}

func TestToolCompatibilityGating(t *testing.T) {
	legacy := &gateway.WorkspaceView{ProtocolVersion: 1}
	if err := ensureToolSupported(definitionNamed(t, "git_status"), legacy); err != nil {
		t.Fatalf("legacy git_status should stay available: %v", err)
	}
	if err := ensureToolSupported(definitionNamed(t, "git_commit"), legacy); err == nil {
		t.Fatal("legacy protocol-v1 client must not receive unsupported git_commit")
	}

	modern := &gateway.WorkspaceView{ProtocolVersion: 2, Capabilities: map[string]any{
		"filesystem":      true,
		"git":             true,
		"shell":           true,
		"pty":             false,
		"mcpHub":          false,
		"approvalMemory":  true,
		"terminalHistory": true,
	}}
	if err := ensureToolSupported(definitionNamed(t, "git_diff"), modern); err != nil {
		t.Fatalf("git_diff should be available: %v", err)
	}
	if err := ensureToolSupported(definitionNamed(t, "pty_start"), modern); err == nil {
		t.Fatal("pty_start must be gated when PTY is not advertised")
	}
	if err := ensureToolSupported(definitionNamed(t, "mcp_call"), modern); err == nil {
		t.Fatal("mcp_call must be gated when MCP Hub is not advertised")
	}
}

func TestLegacyMCPCompatibilityForcesModernProbeToFallback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		setVersion bool
	}{
		{name: "protocol header", body: `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`, setVersion: true},
		{name: "discover method", body: `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := legacyMCPCompatibility(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(tc.body))
			if tc.setVersion {
				req.Header.Set("Mcp-Protocol-Version", modernMCPProtocolVersion)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("modern probe status = %d, want 400", rec.Code)
			}
			if called {
				t.Fatal("modern probe must not reach the stateful Go transport")
			}
			if !strings.Contains(rec.Body.String(), "Invalid or missing MCP session") {
				t.Fatalf("unexpected fallback response: %s", rec.Body.String())
			}
		})
	}
}

func TestLegacyMCPCompatibilityPassesInitialize(t *testing.T) {
	called := false
	handler := legacyMCPCompatibility(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("legacy initialize should pass through: called=%v status=%d", called, rec.Code)
	}
}

func TestStreamableHTTPListsRegisteredTools(t *testing.T) {
	s := &Service{
		servers:      map[string]*mcp.Server{},
		routes:       map[string]map[string]string{},
		shownUpdates: map[string]map[string]struct{}{},
	}
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.serverFor("test-user")
	}, &mcp.StreamableHTTPOptions{Stateless: false, JSONResponse: true})
	httpServer := httptest.NewServer(legacyMCPCompatibility(stream))
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("connect streamable MCP client: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if len(result.Tools) != len(toolDefinitions()) {
		t.Fatalf("tools/list returned %d tools, want %d", len(result.Tools), len(toolDefinitions()))
	}
}
