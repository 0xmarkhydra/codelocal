package mcpgateway

import (
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/protocol"
)

// operationInvocation is the stable internal operation contract used between
// model-facing MCP tools and the existing granular CodeLocal runtime tools.
//
// RuntimeTool intentionally remains the current protocol-v1/v2 tool name so
// this migration does not require a client protocol change. OperationID is the
// durable semantic identity shared by the compact MCP surface and native runtime.
type operationInvocation struct {
	OperationID       string
	RuntimeTool       string
	Capability        string
	Local             bool
	MutatesState      bool
	Destructive       bool
	OpenWorld         bool
	Idempotent        bool
	TerminalExecution bool
	SideEffecting     bool
}

// runtimeOperationIDs freezes the semantic meaning of every native runtime
// command. Compatibility aliases deliberately resolve to the same stable
// operation ID as their canonical command.
var runtimeOperationIDs = map[string]string{
	"list_devices":           "device.list_active",
	"list_device_identities": "device.list_paired",
	"revoke_device":          "device.revoke",
	"rename_device":          "device.rename",
	"list_workspaces":        "workspace.list",
	"select_workspace":       "workspace.select",
	"workspace_info":         "workspace.info",
	"memory_remember":        "memory.remember",
	"memory_recall":          "memory.recall",
	"learned_skill_list":     "skills.list",

	"project_info":      "project.info",
	"project_map":       "project.map",
	"context_for_task":  "context.task",
	"read_instructions": "project.instructions",
	"list_files":        "search.files",
	"file_info":         "read.info",
	"read_file":         "read.file",
	"read_file_range":   "read.range",
	"read_files":        "read.many",
	"search_code":       "search.text",

	"inspect_dependency": "dependency.inspect",
	"read_dependency":    "dependency.read",
	"search_dependency":  "dependency.search",

	"semantic_info":        "lsp.info",
	"workspace_symbols":    "lsp.workspace_symbols",
	"find_symbol":          "lsp.workspace_symbols",
	"document_symbols":     "lsp.document_symbols",
	"find_definition":      "lsp.definition",
	"find_references":      "lsp.references",
	"find_implementations": "lsp.implementations",
	"get_hover":            "lsp.hover",
	"get_diagnostics":      "lsp.diagnostics",
	"get_callers":          "lsp.callers",
	"get_callees":          "lsp.callees",
	"get_import_graph":     "lsp.import_graph",

	"write_file":           "edit.write",
	"edit_file":            "edit.replace",
	"apply_patch":          "edit.patch",
	"apply_edits":          "edit.apply",
	"format_changed_files": "edit.format",
	"snapshot_diagnostics": "verify.snapshot",
	"verify_changes":       "verify.changes",

	"git_status":       "git.status",
	"git_diff":         "git.diff",
	"git_log":          "git.log",
	"git_show":         "git.show",
	"git_blame":        "git.blame",
	"git_file_history": "git.file_history",
	"git_stage":        "git.stage",
	"git_unstage":      "git.unstage",
	"git_commit":       "git.commit",
	"git_push":         "git.push",

	"sandbox_info":       "security.info",
	"sandbox_smoke_test": "security.smoke_test",

	"terminal_preflight": "terminal.preflight",
	"terminal_history":   "terminal.history",
	"run_command":        "terminal.run",
	"exec_start":         "terminal.start",
	"pty_start":          "terminal.start_pty",

	"exec_poll":     "process.poll",
	"pty_poll":      "process.poll",
	"process_poll":  "process.poll",
	"exec_write":    "process.write",
	"pty_write":     "process.write",
	"process_write": "process.write",
	"pty_resize":    "process.resize",
	"exec_signal":   "process.signal",
	"pty_signal":    "process.signal",
	"exec_kill":     "process.kill",
	"pty_kill":      "process.kill",
	"process_kill":  "process.kill",
	"exec_cancel":   "process.cancel",
	"process_list":  "process.list",

	"approval_list":   "approvals.list",
	"approval_revoke": "approvals.revoke",
	"approval_reset":  "approvals.reset",

	"mcp_list":         "mcp.list",
	"mcp_search_tools": "mcp.search",
	"mcp_tool_info":    "mcp.info",
	"mcp_call":         "mcp.call",

	"browser_status":     "browser.status",
	"browser_open":       "browser.open",
	"browser_snapshot":   "browser.snapshot",
	"browser_find":       "browser.find",
	"browser_click":      "browser.click",
	"browser_fill":       "browser.fill",
	"browser_press":      "browser.press",
	"browser_screenshot": "browser.screenshot",
	"browser_console":    "browser.console",
	"browser_requests":   "browser.requests",
	"browser_close":      "browser.close",

	"computer_status":       "computer.status",
	"computer_list_windows": "computer.list_windows",
	"computer_ui_tree":      "computer.ui_tree",
	"computer_screenshot":   "computer.screenshot",
	"computer_focus":        "computer.focus",
	"computer_click":        "computer.click",
	"computer_type":         "computer.type",
	"computer_key":          "computer.key",
	"computer_scroll":       "computer.scroll",
	"computer_drag":         "computer.drag",
	"computer_run":          "computer.run",
}

func runtimeOperationID(name string) (string, bool) {
	id, ok := runtimeOperationIDs[name]
	return id, ok
}

func localOperationTool(name string) bool {
	switch name {
	case "list_devices", "list_device_identities", "revoke_device", "rename_device", "list_workspaces", "select_workspace", "workspace_info", "memory_remember", "memory_recall":
		return true
	default:
		return false
	}
}

func runtimeTerminalExecutionTool(name string) bool {
	switch name {
	case "run_command", "exec_start", "pty_start":
		return true
	default:
		return false
	}
}

func runtimeToolMutatesState(name string) bool {
	if protocol.SideEffecting(name) {
		return true
	}
	switch name {
	case "select_workspace", "memory_remember", "revoke_device", "rename_device", "exec_write", "pty_write", "process_write", "exec_signal", "pty_signal", "exec_kill", "pty_kill", "process_kill":
		return true
	default:
		return false
	}
}

func runtimeToolDestructive(name string) bool {
	switch name {
	case "write_file", "edit_file", "apply_patch", "apply_edits", "format_changed_files", "run_command", "exec_start", "pty_start", "revoke_device", "approval_revoke", "approval_reset", "mcp_call",
		"browser_click", "browser_fill", "browser_press", "computer_focus", "computer_click", "computer_type", "computer_key", "computer_scroll", "computer_drag", "computer_run":
		return true
	default:
		return false
	}
}

func runtimeToolOpenWorld(name string) bool {
	switch name {
	case "run_command", "exec_start", "pty_start", "git_push", "mcp_call", "browser_open":
		return true
	default:
		return false
	}
}

func runtimeToolIdempotent(name string) bool {
	switch name {
	case "select_workspace", "memory_remember", "write_file", "git_stage", "git_unstage", "approval_reset", "revoke_device":
		return true
	default:
		return false
	}
}

func runtimeToolCapability(name string) string {
	if strings.HasPrefix(name, "browser_") {
		return "browser"
	}
	if strings.HasPrefix(name, "computer_") {
		return "computer"
	}
	if strings.HasPrefix(name, "git_") {
		return "git"
	}
	switch name {
	case "run_command", "exec_start", "exec_poll", "exec_write", "exec_signal", "exec_kill", "exec_cancel", "process_poll", "process_list", "process_write", "process_kill", "terminal_preflight":
		return "shell"
	case "terminal_history":
		return "terminalHistory"
	case "pty_start", "pty_poll", "pty_write", "pty_resize", "pty_signal", "pty_kill":
		return "pty"
	case "mcp_list", "mcp_search_tools", "mcp_tool_info", "mcp_call":
		return "mcpHub"
	case "approval_list", "approval_revoke", "approval_reset":
		return "approvalMemory"
	case "learned_skill_list":
		return "learnedSkills"
	default:
		return "filesystem"
	}
}

func operationForRuntimeTool(name string) (operationInvocation, error) {
	id, ok := runtimeOperationID(name)
	if !ok {
		return operationInvocation{}, fmt.Errorf("unregistered CodeLocal operation for tool %s", name)
	}
	return operationInvocation{
		OperationID:       id,
		RuntimeTool:       name,
		Capability:        runtimeToolCapability(name),
		Local:             localOperationTool(name),
		MutatesState:      runtimeToolMutatesState(name),
		Destructive:       runtimeToolDestructive(name),
		OpenWorld:         runtimeToolOpenWorld(name),
		Idempotent:        runtimeToolIdempotent(name),
		TerminalExecution: runtimeTerminalExecutionTool(name),
		SideEffecting:     protocol.SideEffecting(name),
	}, nil
}
