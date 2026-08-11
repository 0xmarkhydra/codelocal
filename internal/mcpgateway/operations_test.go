package mcpgateway

import "testing"

var frozenRuntimeToolNames = []string{
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

func TestEveryRuntimeToolResolvesToStableOperation(t *testing.T) {
	seen := map[string]struct{}{}
	for _, name := range frozenRuntimeToolNames {
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("duplicate runtime tool: %s", name)
		}
		seen[name] = struct{}{}
		operation, err := operationForRuntimeTool(name)
		if err != nil {
			t.Fatalf("%s has no internal operation: %v", name, err)
		}
		if operation.OperationID == "" || operation.RuntimeTool != name {
			t.Fatalf("invalid operation mapping for %s: %#v", name, operation)
		}
	}
	if len(runtimeOperationIDs) != len(frozenRuntimeToolNames) {
		t.Fatalf("operation mapping count = %d, want %d", len(runtimeOperationIDs), len(frozenRuntimeToolNames))
	}
	for name := range runtimeOperationIDs {
		if _, ok := seen[name]; !ok {
			t.Fatalf("unfrozen runtime operation: %s", name)
		}
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
		first, err := operationForRuntimeTool(group[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range group[1:] {
			operation, err := operationForRuntimeTool(name)
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
		operation, err := operationForRuntimeTool(tc.tool)
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
		operation, err := operationForRuntimeTool(tool)
		if err != nil {
			t.Fatal(err)
		}
		if !operation.MutatesState || !operation.SideEffecting {
			t.Fatalf("%s must be serialized and treated as side-effecting: %#v", tool, operation)
		}
	}
}
