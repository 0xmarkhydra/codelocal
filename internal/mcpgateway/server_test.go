package mcpgateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestToolCompatibilityGating(t *testing.T) {
	protocolOne := &gateway.WorkspaceView{ProtocolVersion: 1}
	gitStatus, _ := operationForRuntimeTool("git_status")
	if err := ensureOperationSupported(gitStatus, protocolOne); err != nil {
		t.Fatalf("protocol-v1 git status should stay available: %v", err)
	}
	gitCommit, _ := operationForRuntimeTool("git_commit")
	if err := ensureOperationSupported(gitCommit, protocolOne); err == nil {
		t.Fatal("protocol-v1 client must not receive unsupported git commit")
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
	gitDiff, _ := operationForRuntimeTool("git_diff")
	if err := ensureOperationSupported(gitDiff, modern); err != nil {
		t.Fatalf("git_diff should be available: %v", err)
	}
	ptyStart, _ := operationForRuntimeTool("pty_start")
	if err := ensureOperationSupported(ptyStart, modern); err == nil {
		t.Fatal("pty_start must be gated when PTY is not advertised")
	}
	mcpCall, _ := operationForRuntimeTool("mcp_call")
	if err := ensureOperationSupported(mcpCall, modern); err == nil {
		t.Fatal("mcp_call must be gated when MCP Hub is not advertised")
	}
}

func TestHybridMCPTransportRoutesModernRequestsStateless(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		body       string
		setVersion bool
	}{
		{name: "protocol header post", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, setVersion: true},
		{name: "protocol header get", method: http.MethodGet, setVersion: true},
		{name: "discover method", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`},
		{name: "request meta", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statefulCalled := false
			statelessCalled := false
			handler := hybridMCPTransport(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					statefulCalled = true
					w.WriteHeader(http.StatusNoContent)
				}),
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					statelessCalled = true
					w.WriteHeader(http.StatusNoContent)
				}),
			)
			req := httptest.NewRequest(tc.method, "/mcp", strings.NewReader(tc.body))
			if tc.setVersion {
				req.Header.Set("Mcp-Protocol-Version", modernMCPProtocolVersion)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if !statelessCalled || statefulCalled {
				t.Fatalf("modern request routed incorrectly: stateful=%v stateless=%v", statefulCalled, statelessCalled)
			}
			if got := rec.Header().Get("X-CodeLocal-MCP-Transport"); got != "stateless-2026" {
				t.Fatalf("transport header = %q, want stateless-2026", got)
			}
		})
	}
}

func TestHybridMCPTransportKeepsLegacyInitializeStateful(t *testing.T) {
	statefulCalled := false
	statelessCalled := false
	handler := hybridMCPTransport(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			statefulCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			statelessCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !statefulCalled || statelessCalled {
		t.Fatalf("legacy initialize routed incorrectly: stateful=%v stateless=%v", statefulCalled, statelessCalled)
	}
}

func newTestStreamableMCPHandler() http.Handler {
	s := &Service{
		servers:      map[string]*mcp.Server{},
		routes:       map[string]map[string]string{},
		shownUpdates: map[string]map[string]struct{}{},
	}
	return streamableMCPHandler(func(*http.Request) *mcp.Server {
		return s.serverFor("test-user")
	})
}

func TestStreamableHTTPListsRegisteredTools(t *testing.T) {
	httpServer := httptest.NewServer(newTestStreamableMCPHandler())
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
	if len(result.Tools) != len(compactToolDefinitions()) {
		t.Fatalf("tools/list returned %d tools, want %d", len(result.Tools), len(compactToolDefinitions()))
	}
}

func TestModernStatelessTransportSurvivesGatewayRestart(t *testing.T) {
	var mu sync.RWMutex
	current := newTestStreamableMCPHandler()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		handler := current
		mu.RUnlock()
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("connect streamable MCP client: %v", err)
	}
	defer session.Close()
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatalf("initial tools/list failed: %v", err)
	}

	mu.Lock()
	current = newTestStreamableMCPHandler()
	mu.Unlock()

	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatalf("tools/list after gateway restart failed: %v", err)
	}
}

func TestTextResultWrapsTopLevelArrayStructuredContent(t *testing.T) {
	result := textResult([]any{map[string]any{"windowId": "ax:123:0"}}, false)
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %T, want object", result.StructuredContent)
	}
	items, ok := root["result"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("wrapped structured result = %#v", root)
	}
}

func TestComputerCanInferOneActiveDeviceAcrossMultipleWorkspaces(t *testing.T) {
	computer, _ := operationForRuntimeTool("computer_list_windows")
	active := []gateway.WorkspaceView{
		{Key: "workspace-a", DeviceID: "mac-1", LastSeenAt: 10},
		{Key: "workspace-b", DeviceID: "mac-1", LastSeenAt: 20},
	}
	key, ok := activeWorkspaceKeyForOperation(active, computer)
	if !ok || key != "workspace-b" {
		t.Fatalf("computer route = (%q,%v), want most recent workspace on the single active device", key, ok)
	}

	gitStatus, _ := operationForRuntimeTool("git_status")
	if key, ok := activeWorkspaceKeyForOperation(active, gitStatus); ok || key != "" {
		t.Fatalf("project-scoped route must remain explicit across multiple workspaces: (%q,%v)", key, ok)
	}

	active[1].DeviceID = "mac-2"
	if key, ok := activeWorkspaceKeyForOperation(active, computer); ok || key != "" {
		t.Fatalf("computer route must remain explicit across multiple devices: (%q,%v)", key, ok)
	}
}

func TestSuccessfulResultsCarryWorkspaceHandle(t *testing.T) {
	result := textResult([]any{map[string]any{"windowId": "ax:123:0"}}, false)
	workspace := &gateway.WorkspaceView{Key: "user::device::workspace", DeviceID: "device-1", WorkspaceID: "workspace-1"}
	attachWorkspaceHandle(result, workspace)
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %T, want object", result.StructuredContent)
	}
	if root["workspaceKey"] != workspace.Key || root["deviceId"] != workspace.DeviceID || root["workspaceId"] != workspace.WorkspaceID {
		t.Fatalf("workspace handle missing from successful result: %#v", root)
	}
}

func TestGatewayFailureReasonCodesAreMachineReadable(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		runtimeCode string
		wantCode    string
		wantRetry   bool
	}{
		{name: "runtime route loss", err: errors.New("workspace connection is not owned by this gateway"), runtimeCode: "CLIENT_OFFLINE", wantCode: codeLocalTransientRoutingFailure, wantRetry: true},
		{name: "device offline", err: errors.New("the device for BIDDI is offline; run `codelocal` on that device"), wantCode: codeLocalDeviceOffline},
		{name: "workspace unauthorized", err: errors.New("workspace is not authorized by the local CodeLocal runtime: BIDDI"), wantCode: codeLocalWorkspaceUnauthorized},
		{name: "route missing", err: workspaceRoutingError(false), wantCode: codeLocalWorkspaceNotSelected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, retryable := classifyGatewayFailure(tc.err, tc.runtimeCode)
			if code != tc.wantCode || retryable != tc.wantRetry {
				t.Fatalf("classification = (%q,%v), want (%q,%v)", code, retryable, tc.wantCode, tc.wantRetry)
			}
		})
	}

	result := gatewayFailureResult(errors.New("workspace is offline"), "", "req-1", "workspace-1", 1, true)
	root, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %T, want object", result.StructuredContent)
	}
	if root["code"] != codeLocalTransientRoutingFailure || root["retryable"] != true || root["requestId"] != "req-1" || root["rebound"] != true {
		t.Fatalf("unexpected structured failure: %#v", root)
	}
}

func TestRouteRetryPolicyPreservesSafetyBoundary(t *testing.T) {
	readOnly, _ := operationForRuntimeTool("git_status")
	if !routeRetryAllowed(readOnly, nil) {
		t.Fatal("read-only operations should retry one transient route loss")
	}

	idempotentWrite, _ := operationForRuntimeTool("write_file")
	if !routeRetryAllowed(idempotentWrite, &gateway.WorkspaceView{ProtocolVersion: 1}) {
		t.Fatal("intrinsically idempotent operation should be retryable")
	}

	command, _ := operationForRuntimeTool("run_command")
	legacy := &gateway.WorkspaceView{ProtocolVersion: 1, Capabilities: map[string]any{"idempotency": false}}
	if routeRetryAllowed(command, legacy) {
		t.Fatal("non-idempotent legacy side effect must not be retried")
	}
	modern := &gateway.WorkspaceView{ProtocolVersion: 3, Capabilities: map[string]any{"idempotency": true}}
	if !routeRetryAllowed(command, modern) {
		t.Fatal("modern request-id-idempotent side effect should survive one route rebind")
	}
}
