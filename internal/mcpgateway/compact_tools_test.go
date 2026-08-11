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
	"device", "workspace", "project", "context", "read", "search", "dependency", "lsp",
	"edit", "verify", "git", "terminal", "process", "approvals", "security", "mcp",
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
	if len(defs) != 16 {
		t.Fatalf("compact tool count = %d, want 16 until handoff phase lands", len(defs))
	}
	legacyBytes := legacySurfaceBytes(toolDefinitions())
	compactBytes := compactSurfaceBytes(defs)
	if compactBytes >= legacyBytes {
		t.Fatalf("compact schema should be smaller: compact=%d legacy=%d", compactBytes, legacyBytes)
	}
	t.Logf("MCP surface bytes: legacy=%d compact=%d reduction=%.1f%%", legacyBytes, compactBytes, 100*(1-float64(compactBytes)/float64(legacyBytes)))
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
	}
}

func TestCompactSurfaceCoversEveryLegacyOperation(t *testing.T) {
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
	for _, operationID := range legacyOperationIDs {
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
}

func TestConfiguredToolSurfaceDefaultsSafe(t *testing.T) {
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", "")
	if got := configuredToolSurface(); got != toolSurfaceLegacy {
		t.Fatalf("default surface = %q, want legacy", got)
	}
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", "compact")
	if got := configuredToolSurface(); got != toolSurfaceCompact {
		t.Fatalf("compact surface = %q", got)
	}
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", "dual")
	if got := configuredToolSurface(); got != toolSurfaceDual {
		t.Fatalf("dual surface = %q", got)
	}
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", "garbage")
	if got := configuredToolSurface(); got != toolSurfaceLegacy {
		t.Fatalf("unknown surface must fail closed to legacy, got %q", got)
	}
}

func listServerToolsForSurface(t *testing.T, surface string) []string {
	t.Helper()
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", surface)
	s := &Service{servers: map[string]*mcp.Server{}, routes: map[string]map[string]string{}, shownUpdates: map[string]map[string]struct{}{}}
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.serverFor("test-user-" + surface) }, &mcp.StreamableHTTPOptions{Stateless: false, JSONResponse: true})
	httpServer := httptest.NewServer(legacyMCPCompatibility(stream))
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-surface-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("connect %s surface: %v", surface, err)
	}
	defer session.Close()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list %s tools: %v", surface, err)
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

func TestServerAdvertisesSelectedToolSurface(t *testing.T) {
	compact := listServerToolsForSurface(t, toolSurfaceCompact)
	if !reflect.DeepEqual(sortedStrings(compact), sortedStrings(frozenCompactToolNames)) {
		t.Fatalf("compact advertised tools mismatch: %v", compact)
	}
	dual := listServerToolsForSurface(t, toolSurfaceDual)
	if len(dual) != len(frozenLegacyToolNames)+len(frozenCompactToolNames) {
		t.Fatalf("dual advertised %d tools, want %d", len(dual), len(frozenLegacyToolNames)+len(frozenCompactToolNames))
	}
}

func TestCompactToolCallRunsThroughMCPServer(t *testing.T) {
	t.Setenv("CODELOCAL_MCP_TOOL_SURFACE", toolSurfaceCompact)
	s := &Service{
		Hub:          gateway.NewHub(nil, "compact-call-test"),
		servers:      map[string]*mcp.Server{},
		routes:       map[string]map[string]string{},
		shownUpdates: map[string]map[string]struct{}{},
	}
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.serverFor("test-user")
	}, &mcp.StreamableHTTPOptions{Stateless: false, JSONResponse: true})
	httpServer := httptest.NewServer(legacyMCPCompatibility(stream))
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

func TestRepresentativeCompactCallsMatchLegacyOperations(t *testing.T) {
	definitions := map[string]compactToolDef{}
	for _, definition := range compactToolDefinitions() {
		definitions[definition.Name] = definition
	}
	cases := []struct {
		tool       string
		action     string
		legacyTool string
		args       map[string]any
	}{
		{tool: "workspace", action: "select", legacyTool: "select_workspace", args: map[string]any{"key": "workspace"}},
		{tool: "lsp", action: "definition", legacyTool: "find_definition", args: map[string]any{"path": "main.go", "line": 10, "column": 3}},
		{tool: "edit", action: "apply", legacyTool: "apply_edits", args: map[string]any{"files": []any{map[string]any{"path": "main.go", "edits": []any{}}}}},
		{tool: "terminal", action: "start_pty", legacyTool: "pty_start", args: map[string]any{"command": "go test ./..."}},
		{tool: "process", action: "poll", legacyTool: "exec_poll", args: map[string]any{"processId": "process"}},
		{tool: "git", action: "push", legacyTool: "git_push", args: map[string]any{"remote": "origin", "branch": "dev"}},
		{tool: "mcp", action: "call", legacyTool: "mcp_call", args: map[string]any{"server": "github", "tool": "search", "arguments": map[string]any{}}},
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
			legacy, err := operationForLegacyTool(tc.legacyTool)
			if err != nil {
				t.Fatal(err)
			}
			if operation != legacy {
				t.Fatalf("compact operation = %#v, legacy operation = %#v", operation, legacy)
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
	operation, err := operationForLegacyTool("read_file")
	if err != nil {
		t.Fatal(err)
	}
	legacy := workspaceRoutingError("read_file", operation, false).Error()
	if !strings.Contains(legacy, "list_workspaces") || strings.Contains(legacy, "workspace(action=") {
		t.Fatalf("legacy guidance names unavailable compact tools: %q", legacy)
	}
	compact := workspaceRoutingError("read", operation, false).Error()
	if !strings.Contains(compact, "workspace(action=list)") || strings.Contains(compact, "list_workspaces") {
		t.Fatalf("compact guidance names unavailable legacy tools: %q", compact)
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
