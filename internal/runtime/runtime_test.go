package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestPostReturnsJSONMarshalError(t *testing.T) {
	runtime := New(Options{BaseURL: "http://127.0.0.1"})
	err := runtime.post(context.Background(), "/unused", map[string]any{"invalid": make(chan struct{})}, nil)
	if err == nil {
		t.Fatal("expected JSON marshal error")
	}
}

func TestRegistrySignatureIgnoresWorkspaceOrder(t *testing.T) {
	first := []workspace.Workspace{
		{WorkspaceID: "b", WorkspaceName: "Beta", LocalPath: "/tmp/b"},
		{WorkspaceID: "a", WorkspaceName: "Alpha", LocalPath: "/tmp/a"},
	}
	second := []workspace.Workspace{first[1], first[0]}
	if registrySignature(first) != registrySignature(second) {
		t.Fatal("registry signature should be stable regardless of workspace order")
	}
}

func TestProjectIdentityRefreshDue(t *testing.T) {
	now := time.Now().UnixMilli()
	if !projectIdentityRefreshDue(0, now) {
		t.Fatal("first project identity scan must run")
	}
	if projectIdentityRefreshDue(now-projectIdentityRefreshInterval.Milliseconds()+1, now) {
		t.Fatal("project identity should not rescan before the bounded refresh interval")
	}
	if !projectIdentityRefreshDue(now-projectIdentityRefreshInterval.Milliseconds(), now) {
		t.Fatal("project identity must refresh when the interval elapses so newly added nested repos are discovered")
	}
}

func TestWorkspaceWorkerStopIsConcurrentSafe(t *testing.T) {
	runtime := New(Options{})
	worker := &WorkspaceWorker{
		Runtime:   runtime,
		Workspace: workspace.Workspace{WorkspaceID: "test-workspace"},
		done:      make(chan struct{}),
		calls:     map[string]context.CancelFunc{},
	}
	runtime.workers[worker.Workspace.WorkspaceID] = worker

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker.Stop("concurrent stop")
		}()
	}
	wg.Wait()

	select {
	case <-worker.done:
	default:
		t.Fatal("worker done channel was not closed")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.workers[worker.Workspace.WorkspaceID] != nil {
		t.Fatal("worker was not removed from runtime")
	}
}
