package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/identity"
	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
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

func TestSyncRegistryKeepsProjectBrainOffStartupPayloadAndFailsOpenOnLocalBrainState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CODELOCAL_STATE_DIR", stateDir)
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte("Use repository interfaces.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/workspaces/sync" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode sync body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"synced":1,"removed":0,"knowledge":{},"projectBrain":{"cloudSyncEnabled":false}}`))
	}))
	defer server.Close()

	runtime := New(Options{BaseURL: server.URL, Credential: identity.Credential{CredentialID: "cld_test", CredentialSecret: "secret", DeviceID: "device-a"}})
	if _, err := runtime.Registry.Grant(projectDir, "project-a"); err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(stateDir, "project-brain", "corrupt.json")
	if err := os.MkdirAll(filepath.Dir(corruptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corruptPath, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.brainSync = projectbrain.NewSyncStateStoreAt(corruptPath)

	items, err := runtime.SyncRegistry(context.Background(), true)
	if err != nil || len(items) != 1 {
		t.Fatalf("core registry sync must stay usable when local brain state is corrupt: items=%d err=%v", len(items), err)
	}
	workspaces, ok := body["workspaces"].([]any)
	if !ok || len(workspaces) != 1 {
		t.Fatalf("unexpected workspace payload: %#v", body)
	}
	entry, ok := workspaces[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected workspace entry: %#v", workspaces[0])
	}
	if _, exists := entry["knowledgeManifest"]; exists {
		t.Fatalf("startup registry payload still contains full knowledge manifest: %#v", entry)
	}
	if _, exists := entry["knowledgeDelta"]; exists {
		t.Fatalf("startup registry payload contains Project Brain delta: %#v", entry)
	}
	if runtime.projectBrainCloudEnabled() {
		t.Fatal("server control-plane rollout flag did not disable Project Brain cloud sync")
	}
}

func TestApplyDeltaProvenanceDoesNotChangeManifestIdentity(t *testing.T) {
	source := projectbrain.Source{
		Path: "backend/AGENTS.md", Provider: "agents", SourceType: "instructions", ScopePath: "backend", Classification: "private_project",
		ContentHash: "hash", ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1",
	}
	identity := projectidentity.Snapshot{Repositories: []projectidentity.Repository{{ID: "repo-backend", RelativePath: "backend", IdentitySource: "remote"}}}
	before := projectbrain.SourceFingerprint(source)
	delta := applyDeltaProvenance(projectbrain.ManifestDelta{RootHash: "root", Sources: []projectbrain.Source{source}}, identity, map[string]gitProvenance{"repo-backend": {Branch: "feat/payment", Commit: "abc123"}})
	if len(delta.Sources) != 1 || delta.Sources[0].Branch != "feat/payment" || delta.Sources[0].GitCommit != "abc123" {
		t.Fatalf("branch/commit provenance missing: %#v", delta)
	}
	if projectbrain.SourceFingerprint(delta.Sources[0]) != before {
		t.Fatal("observation provenance changed source content fingerprint")
	}
}
