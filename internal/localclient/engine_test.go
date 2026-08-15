package localclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/protocol"
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

func TestLearnedSkillPrivateRuntimeOperations(t *testing.T) {
	engine := newTestEngine(t)
	steps := []any{
		map[string]any{"tool": "terminal", "args": map[string]any{"action": "run", "command": "xcrun simctl launch booted vn.infivision.biddi.dev"}},
		map[string]any{"tool": "computer", "args": map[string]any{"action": "observe"}},
	}
	recorded, err := engine.Handle(context.Background(), "learned_skill_record", map[string]any{
		"intent": "mở BIDDI Beta", "taskKind": "desktop", "steps": steps, "verified": true,
	}, HandleOptions{RequestID: "skill-record"})
	if err != nil {
		t.Fatal(err)
	}
	recordState, ok := recorded.(map[string]any)
	if !ok || recordState["recipe"] == nil {
		t.Fatalf("unexpected learned skill record result: %#v", recorded)
	}

	matched, err := engine.Handle(context.Background(), "learned_skill_match", map[string]any{
		"intent": "mở app BIDDI Beta", "taskKind": "desktop",
	}, HandleOptions{RequestID: "skill-match"})
	if err != nil {
		t.Fatal(err)
	}
	matchState, ok := matched.(map[string]any)
	if !ok || matchState["match"] == nil {
		t.Fatalf("expected private learned skill match, got %#v", matched)
	}

	listed, err := engine.Handle(context.Background(), "learned_skill_list", map[string]any{"limit": 20}, HandleOptions{RequestID: "skill-list"})
	if err != nil {
		t.Fatal(err)
	}
	listState, ok := listed.(map[string]any)
	if !ok || listState["source"] != "local" {
		t.Fatalf("unexpected learned skill list result: %#v", listed)
	}
	items, ok := listState["skills"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one learned skill metadata item, got %#v", listState["skills"])
	}
	if items[0]["intent"] != "mở BIDDI Beta" || items[0]["stepCount"] != 2 || items[0]["source"] != "local" {
		t.Fatalf("unexpected learned skill metadata: %#v", items[0])
	}
	if items[0]["steps"] != nil {
		t.Fatalf("workspace learned-skill inspection must not expose replay step bodies: %#v", items[0])
	}
}

func TestProjectInfoReportsCurrentProtocolVersion(t *testing.T) {
	engine := newTestEngine(t)
	result, err := engine.Handle(context.Background(), "project_info", nil, HandleOptions{RequestID: "project-info"})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("project_info returned %T, want map", result)
	}
	if got := asInt(info["protocolVersion"], 0); got != protocol.Version {
		t.Fatalf("project_info protocolVersion = %d, want %d", got, protocol.Version)
	}
	capabilities, ok := info["capabilities"].([]string)
	if !ok {
		t.Fatalf("project_info capabilities = %#v, want []string", info["capabilities"])
	}
	want := fmt.Sprintf("protocol-v%d", protocol.Version)
	for _, capability := range capabilities {
		if capability == want {
			return
		}
	}
	t.Fatalf("project_info capabilities = %#v, missing %q", capabilities, want)
}
