package localclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	engine, err := New(root, "workspace-test", "Workspace Test", "device::workspace-test", "device")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine
}

func TestShellDisabledBlocksTerminalTools(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "0")
	engine := newTestEngine(t)
	if engine.ShellEnabled {
		t.Fatal("shell should be disabled")
	}
	preflight, err := engine.Preflight("echo hello", ".")
	if err != nil {
		t.Fatal(err)
	}
	if preflight["status"] != "blocked" {
		t.Fatalf("preflight=%#v", preflight)
	}
	if _, err := engine.Handle(context.Background(), "exec_start", map[string]any{"command": "echo hello"}, HandleOptions{RequestID: "r1"}); err == nil {
		t.Fatal("exec_start should fail when shell is disabled")
	}
}

func TestMCPCallAlwaysRequiresFreshChatApproval(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "1")
	engine := newTestEngine(t)
	options := HandleOptions{RequestID: "mcp-1", SessionID: "s1", IdempotencyKey: "mcp-idem"}
	result, err := engine.Handle(context.Background(), "mcp_call", map[string]any{"server": "external", "tool": "do_thing", "arguments": map[string]any{}}, options)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := result.(map[string]any)
	if !ok || asString(state["status"]) != "approval_required" || asString(state["approvalPolicy"]) != "always" {
		t.Fatalf("unexpected MCP approval state: %#v", result)
	}
	firstToken, _ := state["approvalToken"].(string)
	if firstToken == "" {
		t.Fatal("approval token missing")
	}
	options.RequestID = "mcp-2"
	retry, err := engine.Handle(context.Background(), "mcp_call", map[string]any{"server": "external", "tool": "do_thing", "arguments": map[string]any{}}, options)
	if err != nil {
		t.Fatal(err)
	}
	retryState, ok := retry.(map[string]any)
	if !ok || retryState["status"] != "approval_required" {
		t.Fatalf("approval-required request was incorrectly journaled: %#v", retry)
	}
	if retryState["approvalToken"] == firstToken {
		t.Fatal("short-lived approval token was replayed from idempotency journal")
	}
}

func TestSideEffectingToolIdempotencyReusesCompletedResult(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "1")
	engine := newTestEngine(t)
	path := filepath.Join(engine.Root, "example.txt")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	options := HandleOptions{RequestID: "r1", SessionID: "s1", IdempotencyKey: "idem-1"}
	if _, err := engine.Handle(ctx, "edit_file", map[string]any{"path": "example.txt", "oldText": "first", "newText": "second"}, options); err != nil {
		t.Fatal(err)
	}
	options.RequestID = "r2"
	if _, err := engine.Handle(ctx, "edit_file", map[string]any{"path": "example.txt", "oldText": "second", "newText": "third"}, options); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("idempotent retry changed file: %q", data)
	}
}
