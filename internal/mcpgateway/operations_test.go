package mcpgateway

import (
	"reflect"
	"testing"
)

var frozenLegacyToolNames = []string{
	"list_devices", "list_device_identities", "revoke_device", "rename_device", "list_workspaces", "select_workspace", "workspace_info",
	"project_info", "project_map", "context_for_task", "read_instructions", "list_files", "file_info", "read_file", "read_file_range", "read_files", "search_code",
	"inspect_dependency", "read_dependency", "search_dependency",
	"semantic_info", "workspace_symbols", "find_symbol", "document_symbols", "find_definition", "find_references", "find_implementations", "get_hover", "get_diagnostics", "get_callers", "get_callees", "get_import_graph",
	"write_file", "edit_file", "apply_patch", "apply_edits", "format_changed_files", "snapshot_diagnostics", "verify_changes",
	"git_status", "git_diff", "git_log", "git_show", "git_blame", "git_file_history", "git_stage", "git_unstage", "git_commit", "git_push",
	"sandbox_info", "sandbox_smoke_test", "terminal_preflight", "terminal_history", "run_command", "exec_start", "pty_start",
	"exec_poll", "pty_poll", "process_poll", "exec_write", "pty_write", "process_write", "pty_resize", "exec_signal", "pty_signal", "exec_kill", "pty_kill", "process_kill", "exec_cancel", "process_list",
	"approval_list", "approval_revoke", "approval_reset",
	"mcp_list", "mcp_search_tools", "mcp_tool_info", "mcp_call",
}

func legacySurfaceBytes(defs []toolDef) int {
	total := 0
	for _, def := range defs {
		total += len(def.Name) + len(def.Title) + len(def.Description) + len(def.Schema)
	}
	return total
}

func TestLegacyToolSurfaceContractIsFrozen(t *testing.T) {
	defs := toolDefinitions()
	got := make([]string, 0, len(defs))
	for _, def := range defs {
		got = append(got, def.Name)
	}
	if !reflect.DeepEqual(got, frozenLegacyToolNames) {
		t.Fatalf("legacy MCP tool contract changed\n got: %#v\nwant: %#v", got, frozenLegacyToolNames)
	}
	if len(got) != 77 {
		t.Fatalf("legacy tool count = %d, want 77", len(got))
	}
	t.Logf("legacy MCP surface: tools=%d approximateSchemaBytes=%d", len(defs), legacySurfaceBytes(defs))
}

func TestEveryLegacyToolResolvesToStableOperation(t *testing.T) {
	defs := toolDefinitions()
	seenNames := map[string]struct{}{}
	for _, def := range defs {
		if _, duplicate := seenNames[def.Name]; duplicate {
			t.Fatalf("duplicate legacy tool definition: %s", def.Name)
		}
		seenNames[def.Name] = struct{}{}
		operation, err := operationForLegacyTool(def.Name)
		if err != nil {
			t.Fatalf("%s has no internal operation: %v", def.Name, err)
		}
		if operation.OperationID == "" || operation.RuntimeTool != def.Name {
			t.Fatalf("invalid operation mapping for %s: %#v", def.Name, operation)
		}
		if operation.Local != def.Local {
			t.Fatalf("operation locality mismatch for %s: operation=%v definition=%v", def.Name, operation.Local, def.Local)
		}
	}
	if len(legacyOperationIDs) != len(frozenLegacyToolNames) {
		t.Fatalf("operation mapping count = %d, want %d", len(legacyOperationIDs), len(frozenLegacyToolNames))
	}
}

func TestCompatibilityAliasesShareOperationIDs(t *testing.T) {
	groups := [][]string{
		{"workspace_symbols", "find_symbol"},
		{"exec_poll", "pty_poll", "process_poll"},
		{"exec_write", "pty_write", "process_write"},
		{"exec_signal", "pty_signal"},
		{"exec_kill", "pty_kill", "process_kill"},
	}
	for _, group := range groups {
		first, err := operationForLegacyTool(group[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range group[1:] {
			operation, err := operationForLegacyTool(name)
			if err != nil {
				t.Fatal(err)
			}
			if operation.OperationID != first.OperationID {
				t.Fatalf("compatibility aliases %v must share an operation ID; %s != %s", group, operation.OperationID, first.OperationID)
			}
		}
	}
}

func TestStableOperationMetadataPreservesSafetyHints(t *testing.T) {
	cases := []struct {
		tool        string
		capability  string
		mutates     bool
		destructive bool
		openWorld   bool
	}{
		{tool: "read_file", capability: "filesystem"},
		{tool: "git_status", capability: "git"},
		{tool: "git_push", capability: "git", mutates: true, openWorld: true},
		{tool: "run_command", capability: "shell", mutates: true, destructive: true, openWorld: true},
		{tool: "pty_start", capability: "pty", mutates: true, destructive: true, openWorld: true},
		{tool: "mcp_call", capability: "mcpHub", mutates: true, destructive: true, openWorld: true},
	}
	for _, tc := range cases {
		operation, err := operationForLegacyTool(tc.tool)
		if err != nil {
			t.Fatal(err)
		}
		if operation.Capability != tc.capability || operation.MutatesState != tc.mutates || operation.Destructive != tc.destructive || operation.OpenWorld != tc.openWorld {
			t.Fatalf("unexpected metadata for %s: %#v", tc.tool, operation)
		}
	}
}

func TestProcessMutationAliasesAreSideEffecting(t *testing.T) {
	for _, tool := range []string{"exec_write", "pty_write", "process_write", "pty_resize", "exec_signal", "pty_signal", "exec_kill", "pty_kill", "process_kill"} {
		operation, err := operationForLegacyTool(tool)
		if err != nil {
			t.Fatal(err)
		}
		if !operation.MutatesState || !operation.SideEffecting {
			t.Fatalf("%s must be serialized and treated as side-effecting: %#v", tool, operation)
		}
	}
}

func BenchmarkLegacyToolSurfaceSchema(b *testing.B) {
	defs := toolDefinitions()
	b.ReportMetric(float64(len(defs)), "tools")
	b.ReportMetric(float64(legacySurfaceBytes(defs)), "schema-bytes")
	for i := 0; i < b.N; i++ {
		_ = legacySurfaceBytes(defs)
	}
}
