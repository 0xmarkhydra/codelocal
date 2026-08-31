package mcpgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeEnvelope(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return envelope
}

func callNameAndArgs(t *testing.T, raw []byte) (string, map[string]any) {
	t.Helper()
	envelope := decodeEnvelope(t, raw)
	params, _ := envelope["params"].(map[string]any)
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	return name, args
}

func TestRewriteLegacyToolCallRepresentativeAliases(t *testing.T) {
	tests := []struct {
		legacy string
		want   string
		action string
	}{
		{legacy: "list_devices", want: "workspace", action: "devices"},
		{legacy: "list_device_identities", want: "workspace", action: "paired_devices"},
		{legacy: "select_workspace", want: "workspace", action: "select"},
		{legacy: "read_file", want: "read", action: "file"},
		{legacy: "find_symbol", want: "context", action: "workspace_symbols"},
		{legacy: "edit_file", want: "edit", action: "replace"},
		{legacy: "run_command", want: "terminal", action: "run"},
		{legacy: "pty_poll", want: "terminal", action: "poll"},
		{legacy: "browser_snapshot", want: "browser", action: "snapshot"},
		{legacy: "computer_list_windows", want: "computer", action: "list_windows"},
	}
	for _, tt := range tests {
		t.Run(tt.legacy, func(t *testing.T) {
			raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tt.legacy + `","arguments":{"workspaceKey":"wk"}}}`)
			rewritten, changed := rewriteLegacyToolCall(raw)
			if !changed {
				t.Fatalf("expected %s to be rewritten", tt.legacy)
			}
			name, args := callNameAndArgs(t, rewritten)
			if name != tt.want {
				t.Fatalf("name=%q want %q", name, tt.want)
			}
			if action, _ := args["action"].(string); action != tt.action {
				t.Fatalf("action=%q want %q", action, tt.action)
			}
			if args["workspaceKey"] != "wk" {
				t.Fatalf("workspaceKey was not preserved: %#v", args)
			}
		})
	}
}

func TestRewriteLegacyContextCallAddsTaskAction(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"context_for_task","arguments":{"taskHint":"find bug"}}}`)
	rewritten, changed := rewriteLegacyToolCall(raw)
	if !changed {
		t.Fatal("expected context_for_task to be rewritten")
	}
	name, args := callNameAndArgs(t, rewritten)
	if name != "context" {
		t.Fatalf("name=%q want context", name)
	}
	if action, _ := args["action"].(string); action != "task" {
		t.Fatalf("context compatibility action=%q want task: %#v", action, args)
	}
	if args["taskHint"] != "find bug" {
		t.Fatalf("taskHint was not preserved: %#v", args)
	}
}

func TestRewriteGenerationFourGroupedToolsToGenerationFive(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		wantTool   string
		wantAction string
	}{
		{name: "device", action: "active", wantTool: "workspace", wantAction: "devices"},
		{name: "project", action: "info", wantTool: "context", wantAction: "project_info"},
		{name: "dependency", action: "inspect", wantTool: "context", wantAction: "dependency_inspect"},
		{name: "lsp", action: "definition", wantTool: "context", wantAction: "definition"},
		{name: "process", action: "poll", wantTool: "terminal", wantAction: "poll"},
		{name: "approvals", action: "list", wantTool: "workspace", wantAction: "approvals"},
		{name: "security", action: "info", wantTool: "workspace", wantAction: "security"},
	}
	for _, tt := range tests {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tt.name + `","arguments":{"action":"` + tt.action + `","workspaceKey":"wk"}}}`)
		rewritten, changed := rewriteLegacyToolCall(raw)
		if !changed {
			t.Fatalf("expected generation-4 tool %s to be rewritten", tt.name)
		}
		name, args := callNameAndArgs(t, rewritten)
		if name != tt.wantTool || args["action"] != tt.wantAction || args["workspaceKey"] != "wk" {
			t.Fatalf("rewrite %s(%s) => %s %#v, want %s(%s)", tt.name, tt.action, name, args, tt.wantTool, tt.wantAction)
		}
	}
}

func TestRewriteLegacyToolCallLeavesCompactAndDiscoveryUntouched(t *testing.T) {
	inputs := [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace","arguments":{"action":"devices"}}}`),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`),
	}
	for _, raw := range inputs {
		rewritten, changed := rewriteLegacyToolCall(raw)
		if changed {
			t.Fatalf("unexpected rewrite: %s", rewritten)
		}
		if string(rewritten) != string(raw) {
			t.Fatalf("unchanged payload was modified: got %s want %s", rewritten, raw)
		}
	}
}

func TestLegacyToolCallCompatibilityMiddleware(t *testing.T) {
	var received []byte
	var staleTool string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staleTool = staleToolSchemaFromContext(r.Context())
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read rewritten body: %v", err)
		}
		if r.ContentLength != int64(len(received)) {
			t.Fatalf("content length=%d want %d", r.ContentLength, len(received))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := LegacyToolCallCompatibility(next)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_devices","arguments":{}}}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d", recorder.Code)
	}
	name, args := callNameAndArgs(t, received)
	if name != "workspace" || args["action"] != "devices" {
		t.Fatalf("unexpected rewritten call name=%q args=%#v", name, args)
	}
	if staleTool != "list_devices" {
		t.Fatalf("stale tool marker=%q want list_devices", staleTool)
	}
}

func TestLegacyToolCallCompatibilityRejectsUnknownToolWithReconnectHint(t *testing.T) {
	nextCalled := false
	handler := LegacyToolCallCompatibility(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"browser_magic_old","arguments":{}}}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if nextCalled {
		t.Fatal("unknown tool should be rejected before reaching MCP SDK")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", recorder.Code)
	}
	body := recorder.Body.String()
	for _, want := range []string{"CODELOCAL_TOOL_SCHEMA_MISMATCH", "browser_magic_old", "Reconnect or refresh CodeLocal", "toolSurface"} {
		if !strings.Contains(body, want) {
			t.Fatalf("compatibility response missing %q: %s", want, body)
		}
	}
}
