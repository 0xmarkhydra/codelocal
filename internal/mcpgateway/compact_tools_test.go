package mcpgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var frozenCompactToolNames = []string{
	"device", "workspace", "project", "context", "agent", "read", "search", "dependency", "lsp",
	"edit", "verify", "git", "terminal", "process", "approvals", "security", "mcp", "browser", "computer",
}

func compactSurfaceBytes(defs []compactToolDef) int {
	total := 0
	for _, def := range defs {
		total += len(def.Name) + len(def.Title) + len(def.Description) + len(def.Schema)
	}
	return total
}

func TestCompactToolSurfaceContract(t *testing.T) {
	defs := compactToolDefinitions()
	got := make([]string, 0, len(defs))
	for _, def := range defs {
		got = append(got, def.Name)
	}
	if !reflect.DeepEqual(got, frozenCompactToolNames) {
		t.Fatalf("compact MCP tool contract changed\n got: %#v\nwant: %#v", got, frozenCompactToolNames)
	}
	if len(defs) != 19 {
		t.Fatalf("compact tool count = %d, want 19 including bounded agent orchestration, Browser and Computer Use", len(defs))
	}
	const previousPublicSchemaBytes = 39798
	compactBytes := compactSurfaceBytes(defs)
	if compactBytes >= previousPublicSchemaBytes {
		t.Fatalf("compact schema should be smaller: compact=%d previous=%d", compactBytes, previousPublicSchemaBytes)
	}
	t.Logf("MCP surface bytes: previous=%d compact=%d reduction=%.1f%%", previousPublicSchemaBytes, compactBytes, 100*(1-float64(compactBytes)/float64(previousPublicSchemaBytes)))
}

func allCompactActions(t *testing.T, def compactToolDef) []string {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.Schema, &schema); err != nil {
		t.Fatalf("decode %s schema: %v", def.Name, err)
	}
	return schema.Properties["action"].Enum
}

func universalCompactArgs(action string) map[string]any {
	return map[string]any{
		"action": action, "credentialId": "credential", "deviceName": "Device", "key": "workspace",
		"taskHint": "fix bug", "path": "file.go", "paths": []any{"file.go"}, "startLine": 1, "endLine": 1,
		"name": "Symbol", "query": "query", "line": 1, "column": 1, "limit": 10,
		"content": "content", "oldText": "old", "newText": "new", "patch": "diff --git a/a b/a", "files": []any{map[string]any{"path": "file.go", "edits": []any{map[string]any{"replacement": "x"}}}},
		"message": "commit", "command": "go test ./...", "processId": "process", "input": "input", "cols": 120, "rows": 36,
		"server": "server", "tool": "tool", "id": "approval", "actionKey": "approval-key",
		"url": "https://example.com", "ref": "e1", "text": "input", "windowId": "window-1", "elementId": "element-1",
		"steps": []any{map[string]any{"action": "click", "target": "Save"}},
		"x":     100, "y": 100, "deltaX": 0, "deltaY": 100, "fromX": 10, "fromY": 10, "toX": 100, "toY": 100,
	}
}

func TestCompactSurfaceCoversEveryRuntimeOperation(t *testing.T) {
	covered := map[string]struct{}{}
	for _, def := range compactToolDefinitions() {
		if def.Name == "context" {
			operation, _, err := def.Resolve(map[string]any{"taskHint": "fix bug"})
			if err != nil {
				t.Fatalf("resolve context: %v", err)
			}
			covered[operation.OperationID] = struct{}{}
			continue
		}
		for _, action := range allCompactActions(t, def) {
			operation, _, err := def.Resolve(universalCompactArgs(action))
			if err != nil {
				t.Fatalf("resolve %s(%s): %v", def.Name, action, err)
			}
			covered[operation.OperationID] = struct{}{}
		}
	}

	expected := map[string]struct{}{}
	for _, operationID := range runtimeOperationIDs {
		expected[operationID] = struct{}{}
	}
	if !reflect.DeepEqual(covered, expected) {
		t.Fatalf("compact operation coverage mismatch\ncovered=%v\nexpected=%v", covered, expected)
	}
}

func TestCompactResolverRejectsInvalidOrIncompleteActions(t *testing.T) {
	defs := map[string]compactToolDef{}
	for _, def := range compactToolDefinitions() {
		defs[def.Name] = def
	}
	if _, _, err := defs["git"].Resolve(map[string]any{"action": "commit"}); err == nil {
		t.Fatal("git commit without message must fail before runtime dispatch")
	}
	if _, _, err := defs["approvals"].Resolve(map[string]any{"action": "revoke"}); err == nil {
		t.Fatal("approval revoke without id/actionKey must fail")
	}
	if _, _, err := defs["lsp"].Resolve(map[string]any{"action": "not-real"}); err == nil {
		t.Fatal("unknown compact action must fail")
	}
	if _, _, err := defs["browser"].Resolve(map[string]any{"action": "open"}); err == nil {
		t.Fatal("browser open without url must fail")
	}
	if _, _, err := defs["computer"].Resolve(map[string]any{"action": "click"}); err == nil {
		t.Fatal("computer click without elementId or coordinates must fail")
	}
}

func listServerTools(t *testing.T) []string {
	t.Helper()
	s := &Service{servers: map[string]*mcp.Server{}, routes: map[string]map[string]string{}, shownUpdates: map[string]map[string]struct{}{}}
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.serverFor("test-user") }, &mcp.StreamableHTTPOptions{Stateless: false, JSONResponse: true})
	httpServer := httptest.NewServer(statefulMCPCompatibility(stream))
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-surface-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("connect compact surface: %v", err)
	}
	defer session.Close()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list compact tools: %v", err)
	}
	out := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		out = append(out, tool.Name)
	}
	return out
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func TestServerAdvertisesOnlyCompactToolSurface(t *testing.T) {
	// A stale deployment variable must not be able to re-enable the removed
	// granular public surface.
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", "legacy")
	compact := listServerTools(t)
	if !reflect.DeepEqual(sortedStrings(compact), sortedStrings(frozenCompactToolNames)) {
		t.Fatalf("compact advertised tools mismatch: %v", compact)
	}
	for _, name := range compact {
		if _, internalRuntimeTool := runtimeOperationIDs[name]; internalRuntimeTool {
			t.Fatalf("internal runtime tool %q leaked into the public MCP surface", name)
		}
	}
}

func TestCompactToolCallRunsThroughMCPServer(t *testing.T) {
	s := &Service{
		Hub:          gateway.NewHub(nil, "compact-call-test"),
		servers:      map[string]*mcp.Server{},
		routes:       map[string]map[string]string{},
		shownUpdates: map[string]map[string]struct{}{},
	}
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.serverFor("test-user")
	}, &mcp.StreamableHTTPOptions{Stateless: false, JSONResponse: true})
	httpServer := httptest.NewServer(statefulMCPCompatibility(stream))
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-compact-call-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "device", Arguments: map[string]any{"action": "active"}})
	if err != nil {
		t.Fatalf("call compact device tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("compact device tool returned an error: %#v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("compact result missing structured content: %#v", result.StructuredContent)
	}
	devices, ok := structured["devices"].([]any)
	if !ok || len(devices) != 0 {
		t.Fatalf("unexpected active device payload: %#v", structured)
	}
	if len(result.Content) != 1 {
		t.Fatalf("compact result content count = %d, want 1", len(result.Content))
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok || textContent.Text != `{"devices":[]}` {
		t.Fatalf("compact result should keep compact JSON text compatibility: %#v", result.Content[0])
	}

	invalid, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "device", Arguments: map[string]any{"action": "not-real"}})
	if err != nil {
		t.Fatalf("invalid compact action should be model-visible tool error, got protocol error: %v", err)
	}
	if !invalid.IsError {
		t.Fatal("invalid compact action must return an MCP tool error")
	}
}

func TestRepresentativeCompactCallsMatchRuntimeOperations(t *testing.T) {
	definitions := map[string]compactToolDef{}
	for _, definition := range compactToolDefinitions() {
		definitions[definition.Name] = definition
	}
	cases := []struct {
		tool        string
		action      string
		runtimeTool string
		args        map[string]any
	}{
		{tool: "workspace", action: "select", runtimeTool: "select_workspace", args: map[string]any{"key": "workspace"}},
		{tool: "lsp", action: "definition", runtimeTool: "find_definition", args: map[string]any{"path": "main.go", "line": 10, "column": 3}},
		{tool: "edit", action: "apply", runtimeTool: "apply_edits", args: map[string]any{"files": []any{map[string]any{"path": "main.go", "edits": []any{}}}}},
		{tool: "terminal", action: "start_pty", runtimeTool: "pty_start", args: map[string]any{"command": "go test ./..."}},
		{tool: "process", action: "poll", runtimeTool: "exec_poll", args: map[string]any{"processId": "process"}},
		{tool: "git", action: "push", runtimeTool: "git_push", args: map[string]any{"remote": "origin", "branch": "dev"}},
		{tool: "mcp", action: "call", runtimeTool: "mcp_call", args: map[string]any{"server": "github", "tool": "search", "arguments": map[string]any{}}},
	}
	for _, tc := range cases {
		t.Run(tc.tool+"_"+tc.action, func(t *testing.T) {
			args := cloneArgs(tc.args)
			args["action"] = tc.action
			args["workspaceKey"] = "device::workspace"
			operation, forward, err := definitions[tc.tool].Resolve(args)
			if err != nil {
				t.Fatal(err)
			}
			runtimeOperation, err := operationForRuntimeTool(tc.runtimeTool)
			if err != nil {
				t.Fatal(err)
			}
			if operation != runtimeOperation {
				t.Fatalf("compact operation = %#v, runtime operation = %#v", operation, runtimeOperation)
			}
			if _, leaked := forward["action"]; leaked {
				t.Fatalf("compact discriminator leaked to runtime args: %#v", forward)
			}
			if forward["workspaceKey"] != "device::workspace" {
				t.Fatalf("workspace routing was not preserved: %#v", forward)
			}
		})
	}
}

func TestWorkspaceRoutingErrorsMatchAdvertisedSurface(t *testing.T) {
	compact := workspaceRoutingError(false).Error()
	if !strings.Contains(compact, "workspace(action=list)") || strings.Contains(compact, "list_workspaces") {
		t.Fatalf("compact guidance names removed public tools: %q", compact)
	}
}

func TestGatewayLatencyMetadataAcceptsLocalAndJSONNumbers(t *testing.T) {
	if got := metadataInt64(map[string]any{"runtimeDurationMs": int64(12)}, "runtimeDurationMs"); got != 12 {
		t.Fatalf("local duration = %d, want 12", got)
	}
	if got := metadataInt64(map[string]any{"runtimeDurationMs": float64(34)}, "runtimeDurationMs"); got != 34 {
		t.Fatalf("JSON duration = %d, want 34", got)
	}
}

func BenchmarkCompactToolSurfaceSchema(b *testing.B) {
	defs := compactToolDefinitions()
	b.ReportMetric(float64(len(defs)), "tools")
	b.ReportMetric(float64(compactSurfaceBytes(defs)), "schema-bytes")
	for i := 0; i < b.N; i++ {
		_ = compactSurfaceBytes(defs)
	}
}
