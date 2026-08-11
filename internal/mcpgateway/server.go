package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/clientupdate"
	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/oauth"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	usagecalc "github.com/0xmarkhydra/codelocal/internal/usage"
	"github.com/0xmarkhydra/codelocal/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Service struct {
	Store        *cloud.Store
	Hub          *gateway.Hub
	Workspaces   *gateway.WorkspaceService
	Release      clientupdate.Manifest
	mu           sync.Mutex
	servers      map[string]*mcp.Server
	routes       map[string]map[string]string
	shownUpdates map[string]map[string]struct{}
}

func New(store *cloud.Store, hub *gateway.Hub, workspaces *gateway.WorkspaceService) *Service {
	return &Service{Store: store, Hub: hub, Workspaces: workspaces, Release: clientupdate.ManifestFromEnv(), servers: map[string]*mcp.Server{}, routes: map[string]map[string]string{}, shownUpdates: map[string]map[string]struct{}{}}
}

func objectSchema(properties map[string]any, required ...string) json.RawMessage {
	value := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		value["required"] = required
	}
	raw, _ := json.Marshal(value)
	return raw
}
func str(description string) map[string]any { return map[string]any{"type": "string", "description": description} }
func boolean(description string) map[string]any { return map[string]any{"type": "boolean", "description": description} }
func integer(description string, min, max int) map[string]any {
	out := map[string]any{"type": "integer", "description": description}
	if min != 0 {
		out["minimum"] = min
	}
	if max != 0 {
		out["maximum"] = max
	}
	return out
}
func array(items any, description string) map[string]any {
	return map[string]any{"type": "array", "items": items, "description": description}
}
func anyObject(description string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true, "description": description}
}

var workspaceKeySchema = map[string]any{"type": "string", "minLength": 1, "description": "Exact workspace key returned by select_workspace. Pass it to keep routing explicit across multiple active projects, ChatGPT threads or fresh MCP sessions."}

type toolDef struct {
	Name, Title, Description string
	Schema                   json.RawMessage
	Local                    bool
}

const orchestrationInstructions = `CodeLocal connects ChatGPT to explicitly authorized local workspaces. Use the smallest useful number of tool calls. Reuse the workspace selected for the current MCP session and reuse results already obtained in the conversation. Do not call workspace_info or project_info repeatedly unless the workspace changed or fresh capability/project metadata is required. For coding, debugging, review and refactor tasks, call context_for_task early and treat its semantic/LSP symbols, graph neighbors, ranked files and bounded snippets as the initial context packet. Follow with exact semantic navigation or targeted line reads only when necessary. Use search_code mainly for literal strings, config keys, logs or unknown text rather than as the default repository discovery strategy. Do not call git_status merely as a prerequisite for git_diff when the diff alone answers the question. After edits, prefer verify_changes and targeted diagnostics/tests; run a full test suite only when it materially improves verification. For browser work, use browser_snapshot/browser_find before interaction and prefer structured element refs over coordinates. Browser and Computer Use are separate permission domains; first-run capability consent does not waive action-level approval. Never repeat a successful read, diff, diagnostic, test or automation observation without a concrete reason. Local security policy and ChatGPT approvals remain authoritative for side effects.`

func boolPtr(value bool) *bool { return &value }

func toolDisplayTitle(name, fallback string) string {
	titles := map[string]string{
		"list_devices":           "Show active CodeLocal devices",
		"list_device_identities": "Show paired devices",
		"revoke_device":          "Revoke paired device",
		"rename_device":          "Rename paired device",
		"list_workspaces":        "Show authorized workspaces",
		"select_workspace":       "Choose workspace",
		"workspace_info":         "Show current workspace",
		"project_info":           "Inspect workspace",
		"project_map":            "Scan project structure",
		"context_for_task":       "Find relevant code",
		"read_instructions":      "Read project instructions",
		"list_files":             "Browse project files",
		"file_info":              "Inspect file metadata",
		"read_file":              "Read file",
		"read_file_range":        "Read file section",
		"read_files":             "Read selected files",
		"search_code":            "Search project text",
		"inspect_dependency":     "Inspect dependency",
		"read_dependency":        "Read dependency source",
		"search_dependency":      "Search dependency source",
		"semantic_info":          "Inspect code intelligence",
		"workspace_symbols":      "Find project symbols",
		"find_symbol":            "Find symbol",
		"document_symbols":       "Find file symbols",
		"find_definition":        "Find definition",
		"find_references":        "Find references",
		"find_implementations":   "Find implementations",
		"get_hover":              "Inspect symbol type",
		"get_diagnostics":        "Check code diagnostics",
		"get_callers":            "Find callers",
		"get_callees":            "Find callees",
		"get_import_graph":       "Inspect import graph",
		"write_file":             "Write file",
		"edit_file":              "Edit file",
		"apply_patch":            "Apply code patch",
		"apply_edits":            "Apply structured edits",
		"format_changed_files":   "Format changed files",
		"snapshot_diagnostics":   "Snapshot diagnostics",
		"verify_changes":         "Verify code changes",
		"git_status":             "Check Git status",
		"git_diff":               "Review code changes",
		"git_log":                "Read Git history",
		"git_show":               "Inspect Git commit",
		"git_blame":              "Inspect Git blame",
		"git_file_history":       "Read file history",
		"git_stage":              "Stage Git changes",
		"git_unstage":            "Unstage Git changes",
		"git_commit":             "Commit Git changes",
		"git_push":               "Push Git commits",
		"sandbox_info":           "Inspect execution security",
		"sandbox_smoke_test":     "Check execution security",
		"terminal_preflight":     "Check command safety",
		"terminal_history":       "Read terminal history",
		"run_command":            "Run command",
		"exec_start":             "Start background command",
		"pty_start":              "Start interactive command",
		"exec_poll":              "Read command output",
		"pty_poll":               "Read interactive output",
		"process_poll":           "Read process output",
		"exec_write":             "Write process input",
		"pty_write":              "Write interactive input",
		"process_write":          "Write process input",
		"pty_resize":             "Resize interactive terminal",
		"exec_signal":            "Signal process",
		"pty_signal":             "Signal interactive process",
		"exec_kill":              "Stop process",
		"pty_kill":               "Stop interactive process",
		"process_kill":           "Stop process",
		"exec_cancel":            "Cancel command",
		"process_list":           "Show running commands",
		"approval_list":          "Show remembered approvals",
		"approval_revoke":        "Forget approval",
		"approval_reset":         "Reset approvals",
		"mcp_list":               "Show installed MCP extensions",
		"mcp_search_tools":       "Find MCP extension tools",
		"mcp_tool_info":          "Inspect MCP extension tool",
		"mcp_call":               "Call MCP extension tool",
	}
	if title := titles[name]; title != "" {
		return title
	}
	return fallback
}

func terminalExecutionTool(name string) bool {
	switch name {
	case "run_command", "exec_start", "pty_start":
		return true
	default:
		return false
	}
}

func toolMutatesState(name string) bool {
	if protocol.SideEffecting(name) {
		return true
	}
	switch name {
	case "select_workspace", "revoke_device", "rename_device", "exec_write", "pty_write", "exec_signal", "pty_signal", "exec_kill", "pty_kill", "process_kill":
		return true
	default:
		return false
	}
}

func toolAnnotations(def toolDef) *mcp.ToolAnnotations {
	name := def.Name
	readOnly := !toolMutatesState(name)
	destructive := false
	openWorld := false
	idempotent := false
	switch name {
	case "write_file", "edit_file", "apply_patch", "apply_edits", "format_changed_files", "run_command", "exec_start", "pty_start", "revoke_device", "approval_revoke", "approval_reset", "mcp_call", "browser_click", "browser_fill", "browser_press", "computer_focus", "computer_click", "computer_type", "computer_key", "computer_scroll", "computer_drag":
		destructive = true
	}
	switch name {
	case "run_command", "exec_start", "pty_start", "git_push", "mcp_call", "browser_open":
		openWorld = true
	}
	switch name {
	case "select_workspace", "write_file", "git_stage", "git_unstage", "approval_reset", "revoke_device":
		idempotent = true
	}
	return &mcp.ToolAnnotations{
		Title:           toolDisplayTitle(name, def.Title),
		ReadOnlyHint:    readOnly,
		DestructiveHint: boolPtr(destructive),
		IdempotentHint:  idempotent,
		OpenWorldHint:   boolPtr(openWorld),
	}
}

func legacyProtocolOneTool(name string) bool {
	switch name {
	case "project_info", "read_instructions", "list_files", "file_info", "read_file", "read_file_range", "read_files", "search_code", "inspect_dependency", "read_dependency", "search_dependency", "find_symbol", "find_definition", "find_references", "get_callers", "get_callees", "get_import_graph", "get_diagnostics", "write_file", "edit_file", "apply_patch", "git_status", "git_diff", "git_log", "git_show", "git_blame", "git_file_history", "run_command", "process_poll", "process_list", "process_write", "process_kill":
		return true
	default:
		return false
	}
}

func capabilityBool(capabilities map[string]any, name string) bool {
	value, _ := capabilities[name].(bool)
	return value
}

func toolCapability(name string) string {
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
	default:
		return "filesystem"
	}
}

func ensureToolSupported(def toolDef, workspace *gateway.WorkspaceView) error {
	if workspace == nil {
		return errors.New("workspace unavailable")
	}
	if automationTool(def.Name) {
		return ensureAutomationToolSupported(def, workspace)
	}
	if workspace.ProtocolVersion <= 1 {
		if !legacyProtocolOneTool(def.Name) {
			return fmt.Errorf("%s requires a newer CodeLocal client; update the client before using this tool", def.Name)
		}
		return nil
	}
	capability := toolCapability(def.Name)
	if capability == "filesystem" && (def.Name == "sandbox_info" || def.Name == "sandbox_smoke_test") {
		return nil
	}
	if capability == "pty" {
		if !capabilityBool(workspace.Capabilities, "shell") || !capabilityBool(workspace.Capabilities, "pty") {
			return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise PTY support", def.Name)
		}
		return nil
	}
	if !capabilityBool(workspace.Capabilities, capability) {
		return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise %s support", def.Name, capability)
	}
	return nil
}

func toolDefinitions() []toolDef {
	path := str("Workspace-relative path.")
	approval := str("One-time approval token returned by a prior approval-required result.")
	remote := []toolDef{
		{"project_info", "Project info", "Inspect workspace capabilities, project map, semantic providers, host execution policy, approval memory and instructions. For coding/debug/refactor work, follow with context_for_task before broad scans.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"project_map", "Project map", "Return cached compact project structure, languages, frameworks, commands and roots.", objectSchema(map[string]any{"force": boolean("Force a fresh project scan."), "workspaceKey": workspaceKeySchema}), false},
		{"context_for_task", "Context for task", "Primary semantic-first retrieval step for coding, debugging, review and refactor work. Rank context using LSP/native structural symbols, literal fallback signals and import-graph neighbors, then return symbol-centered bounded snippets. Use this packet before broad repository scans.", objectSchema(map[string]any{"taskHint": str("Concrete coding task used to rank semantic and graph context."), "limit": integer("Maximum ranked results.", 1, 100), "workspaceKey": workspaceKeySchema}, "taskHint"), false},
		{"read_instructions", "Read instructions", "Read scoped AGENTS.md and supported coding instructions.", objectSchema(map[string]any{"path": path, "workspaceKey": workspaceKeySchema}), false},
		{"list_files", "List files", "Gitignore-aware project listing. Sensitive paths remain blocked. For coding tasks, prefer context_for_task first and list files only when structural discovery is still needed.", objectSchema(map[string]any{"path": path, "maxDepth": integer("Maximum directory recursion depth.", 0, 20), "includeIgnored": boolean("Include gitignored entries."), "workspaceKey": workspaceKeySchema}), false},
		{"file_info", "File metadata", "Read metadata/hash without source content.", objectSchema(map[string]any{"path": path, "workspaceKey": workspaceKeySchema}, "path"), false},
		{"read_file", "Read file", "Read a targeted UTF-8 file. For coding tasks, prefer files identified by context_for_task or semantic navigation.", objectSchema(map[string]any{"path": path, "workspaceKey": workspaceKeySchema}, "path"), false},
		{"read_file_range", "Read file range", "Read a targeted line range, preferably around a symbol, definition, reference or diagnostic identified by semantic context.", objectSchema(map[string]any{"path": path, "startLine": integer("1-based first line.", 1, 0), "endLine": integer("1-based last line.", 1, 0), "workspaceKey": workspaceKeySchema}, "path", "startLine", "endLine"), false},
		{"read_files", "Read files", "Batch read targeted files. Avoid broad source dumping; use context_for_task first and expand only ranked files that need more context.", objectSchema(map[string]any{"paths": array(str("Workspace-relative path."), "Files to read."), "workspaceKey": workspaceKeySchema}, "paths"), false},
		{"search_code", "Search code", "Literal-text fallback for exact strings, config keys, logs and unknown text. For coding/debug/refactor discovery, prefer context_for_task and semantic definition/reference tools instead of repository-wide grep.", objectSchema(map[string]any{"query": str("Search query."), "path": path, "maxResults": integer("Maximum matches.", 1, 1000), "fixedStrings": boolean("Treat query literally."), "includeIgnored": boolean("Search ignored files."), "workspaceKey": workspaceKeySchema}, "query"), false},
		{"inspect_dependency", "Inspect dependency", "Inspect dependency metadata for Node/Python/Rust/Go.", objectSchema(map[string]any{"name": str("Dependency/package/module name."), "ecosystem": map[string]any{"type": "string", "enum": []string{"auto", "node", "python", "rust", "go"}}, "workspaceKey": workspaceKeySchema}, "name"), false},
		{"read_dependency", "Read dependency", "Read a targeted installed dependency file.", objectSchema(map[string]any{"name": str("Dependency name."), "path": str("Path inside dependency."), "startLine": integer("First line.", 1, 0), "endLine": integer("Last line.", 1, 0), "ecosystem": str("Dependency ecosystem."), "workspaceKey": workspaceKeySchema}, "name"), false},
		{"search_dependency", "Search dependency", "Search inside one installed dependency.", objectSchema(map[string]any{"name": str("Dependency name."), "query": str("Search query."), "maxResults": integer("Maximum matches.", 1, 500), "fixedStrings": boolean("Literal search."), "workspaceKey": workspaceKeySchema}, "name", "query"), false},
		{"semantic_info", "Semantic providers", "Show installed semantic providers and fallback mode.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"workspace_symbols", "Workspace symbols", "Find symbols across a polyglot workspace.", objectSchema(map[string]any{"query": str("Symbol query."), "limit": integer("Maximum symbols.", 1, 1000), "workspaceKey": workspaceKeySchema}), false},
		{"find_symbol", "Find symbol", "Compatibility alias for workspace symbol lookup.", objectSchema(map[string]any{"query": str("Symbol query."), "limit": integer("Maximum symbols.", 1, 1000), "workspaceKey": workspaceKeySchema}), false},
		{"document_symbols", "Document symbols", "Find structural symbols in a file using LSP/fallback.", objectSchema(map[string]any{"path": path, "limit": integer("Maximum symbols.", 1, 1000), "workspaceKey": workspaceKeySchema}, "path"), false},
		{"find_definition", "Find definition", "Find definitions using exact LSP position when supplied, otherwise name/fallback search.", semanticSchema(), false},
		{"find_references", "Find references", "Find references using exact LSP position when supplied, otherwise name/fallback search.", semanticSchema(), false},
		{"find_implementations", "Find implementations", "Find implementations using the file position semantic provider.", objectSchema(map[string]any{"path": path, "line": integer("1-based line.", 1, 0), "column": integer("1-based column.", 1, 0), "limit": integer("Maximum results.", 1, 1000), "workspaceKey": workspaceKeySchema}, "path", "line", "column"), false},
		{"get_hover", "Get hover", "Get type/signature/hover information at a file position.", objectSchema(map[string]any{"path": path, "line": integer("1-based line.", 1, 0), "column": integer("1-based column.", 1, 0), "workspaceKey": workspaceKeySchema}, "path", "line", "column"), false},
		{"get_diagnostics", "Get diagnostics", "Get diagnostics from the file's semantic provider.", objectSchema(map[string]any{"path": path, "limit": integer("Maximum diagnostics.", 1, 2000), "workspaceKey": workspaceKeySchema}), false},
		{"get_callers", "Get callers", "Get callers where semantic call graph is available.", objectSchema(map[string]any{"name": str("Symbol name."), "limit": integer("Maximum callers.", 1, 1000), "workspaceKey": workspaceKeySchema}, "name"), false},
		{"get_callees", "Get callees", "Get callees where semantic call graph is available.", objectSchema(map[string]any{"name": str("Symbol name."), "limit": integer("Maximum callees.", 1, 1000), "workspaceKey": workspaceKeySchema}, "name"), false},
		{"get_import_graph", "Get import graph", "Get current lightweight import graph.", objectSchema(map[string]any{"limit": integer("Maximum edges.", 1, 10000), "workspaceKey": workspaceKeySchema}), false},
		{"write_file", "Write file", "Create/overwrite a file with optional stale-hash protection.", objectSchema(map[string]any{"path": path, "content": str("Complete file content."), "expectedHash": str("Optional SHA-256 from previous read."), "workspaceKey": workspaceKeySchema}, "path", "content"), false},
		{"edit_file", "Exact edit", "Compatibility exact-text edit with optional hash protection.", objectSchema(map[string]any{"path": path, "oldText": str("Exact text to replace."), "newText": str("Replacement text."), "replaceAll": boolean("Replace all occurrences."), "expectedHash": str("Optional SHA-256 from previous read."), "workspaceKey": workspaceKeySchema}, "path", "oldText", "newText"), false},
		{"apply_patch", "Apply patch", "Validate and apply a unified Git patch.", objectSchema(map[string]any{"patch": str("Unified Git patch."), "workspaceKey": workspaceKeySchema}, "patch"), false},
		{"apply_edits", "Apply structured edits", "Transactionally validate and apply multiple hash-safe file/range edits with rollback on failure.", objectSchema(map[string]any{"files": array(anyObject("File edit transaction."), "Files and edits."), "workspaceKey": workspaceKeySchema}, "files"), false},
		{"format_changed_files", "Format changed files", "Use installed project/language formatters without downloading tools.", objectSchema(map[string]any{"paths": array(str("Path."), "Changed files."), "workspaceKey": workspaceKeySchema}, "paths"), false},
		{"snapshot_diagnostics", "Snapshot diagnostics", "Create a before-change diagnostic baseline.", objectSchema(map[string]any{"paths": array(str("Path."), "Paths to inspect."), "workspaceKey": workspaceKeySchema}), false},
		{"verify_changes", "Verify changes", "Return diagnostics regression, recommended checks and Git diff after edits.", objectSchema(map[string]any{"paths": array(str("Path."), "Changed paths."), "baselineId": str("Optional diagnostic baseline ID."), "workspaceKey": workspaceKeySchema}), false},
		{"git_status", "Git status", "Read branch and working tree status.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"git_diff", "Git diff", "Read working/staged diff.", objectSchema(map[string]any{"cached": boolean("Read staged diff."), "path": path, "workspaceKey": workspaceKeySchema}), false},
		{"git_log", "Git log", "Read recent Git history.", objectSchema(map[string]any{"limit": integer("Commit count.", 1, 100), "path": path, "workspaceKey": workspaceKeySchema}), false},
		{"git_show", "Git show", "Read a commit/ref summary.", objectSchema(map[string]any{"ref": str("Git ref."), "workspaceKey": workspaceKeySchema}), false},
		{"git_blame", "Git blame", "Read blame for a file/range.", objectSchema(map[string]any{"path": path, "startLine": integer("First line.", 1, 0), "endLine": integer("Last line.", 1, 0), "workspaceKey": workspaceKeySchema}, "path"), false},
		{"git_file_history", "Git file history", "Read follow-renames file history.", objectSchema(map[string]any{"path": path, "limit": integer("Commit count.", 1, 100), "workspaceKey": workspaceKeySchema}, "path"), false},
		{"git_stage", "Stage files", "Git stage operation. Reviewed actions require approval.", objectSchema(map[string]any{"paths": array(str("Path."), "Files to stage."), "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "paths"), false},
		{"git_unstage", "Unstage files", "Git unstage operation. Reviewed actions require approval.", objectSchema(map[string]any{"paths": array(str("Path."), "Files to unstage."), "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "paths"), false},
		{"git_commit", "Commit staged changes", "Commit staged changes on the host after policy approval.", objectSchema(map[string]any{"message": str("Commit message."), "expectedPaths": array(str("Path."), "Expected staged paths."), "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "message"), false},
		{"git_push", "Push commits", "Non-force push on the host. Force push remains blocked.", objectSchema(map[string]any{"remote": str("Remote name."), "branch": str("Branch/ref."), "force": boolean("Must remain false."), "approvalToken": approval, "workspaceKey": workspaceKeySchema}), false},
		{"sandbox_info", "Execution security", "Report the active host-policy execution model.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"sandbox_smoke_test", "Execution security smoke test", "Check host-policy execution configuration.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"terminal_preflight", "Check terminal command risk", "Call before terminal execution; returns approval requirements without executing.", objectSchema(map[string]any{"command": str("Shell command."), "cwd": path, "workspaceKey": workspaceKeySchema}, "command"), false},
		{"terminal_history", "Terminal history", "Query local redacted audit history of terminal commands.", objectSchema(map[string]any{"query": str("Optional query."), "limit": integer("Maximum records.", 1, 500), "event": map[string]any{"type": "string", "enum": []string{"started", "finished", "all"}}, "workspaceKey": workspaceKeySchema}), false},
		{"run_command", "Run command", "Run a guarded host command after terminal_preflight/approval.", processSchema(approval, true), false},
		{"exec_start", "Start process", "Start a guarded non-PTY process.", processSchema(approval, false), false},
		{"pty_start", "Start PTY", "Start a guarded PTY process.", processSchema(approval, false), false},
		{"exec_poll", "Poll process", "Read incremental stdout/stderr.", pollSchema(), false}, {"pty_poll", "Poll PTY", "Read incremental PTY output.", pollSchema(), false},
		{"process_poll", "Poll process compatibility", "Compatibility process poll.", objectSchema(map[string]any{"processId": str("Process ID."), "cursor": integer("Byte cursor.", 0, 0), "workspaceKey": workspaceKeySchema}, "processId"), false},
		{"exec_write", "Write process stdin", "Write to a running process stdin.", writeProcessSchema(), false}, {"pty_write", "Write PTY", "Write input to PTY.", writeProcessSchema(), false}, {"process_write", "Write process compatibility", "Compatibility stdin write.", writeProcessSchema(), false},
		{"pty_resize", "Resize PTY", "Resize a true PTY.", objectSchema(map[string]any{"processId": str("Process ID."), "cols": integer("Columns.", 10, 500), "rows": integer("Rows.", 5, 300), "workspaceKey": workspaceKeySchema}, "processId", "cols", "rows"), false},
		{"exec_signal", "Signal process", "Send a supported signal.", signalSchema(), false}, {"pty_signal", "Signal PTY", "Signal PTY/process.", signalSchema(), false}, {"exec_kill", "Kill process", "Terminate a process.", signalSchema(), false}, {"pty_kill", "Kill PTY", "Terminate PTY/process.", signalSchema(), false}, {"process_kill", "Kill process compatibility", "Compatibility termination.", signalSchema(), false},
		{"exec_cancel", "Cancel process", "Cancel a running process.", objectSchema(map[string]any{"processId": str("Process ID."), "reason": str("Cancellation reason."), "workspaceKey": workspaceKeySchema}, "processId"), false}, {"process_list", "List processes", "List CodeLocal-started processes.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"approval_list", "List remembered approvals", "List approval memory stored locally for the selected workspace.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"approval_revoke", "Revoke remembered approval", "Forget one locally remembered approval.", objectSchema(map[string]any{"id": str("Approval ID."), "actionKey": str("Approval action key."), "workspaceKey": workspaceKeySchema}), false},
		{"approval_reset", "Reset remembered approvals", "Forget all approvals for a workspace.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"mcp_list", "List installed MCPs", "List MCP extensions installed in the selected local workspace.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), false},
		{"mcp_search_tools", "Search installed MCP tools", "Search local MCP extension catalog.", objectSchema(map[string]any{"query": str("Search query."), "limit": integer("Maximum results.", 1, 50), "server": str("Optional server name."), "refresh": boolean("Refresh remote catalog."), "workspaceKey": workspaceKeySchema}), false},
		{"mcp_tool_info", "Inspect MCP tool", "Get one installed MCP tool schema.", objectSchema(map[string]any{"server": str("MCP server name."), "tool": str("MCP tool name."), "workspaceKey": workspaceKeySchema}, "server", "tool"), false},
		{"mcp_call", "Call installed MCP tool", "Call one tool from an installed MCP extension; local policy remains authoritative.", objectSchema(map[string]any{"server": str("MCP server name."), "tool": str("MCP tool name."), "arguments": anyObject("Tool arguments."), "approvalToken": approval, "workspaceKey": workspaceKeySchema}, "server", "tool"), false},
	}
	local := []toolDef{{"list_devices", "List active devices", "List active CodeLocal workspace clients belonging to this account.", objectSchema(map[string]any{}), true}, {"list_device_identities", "List paired devices", "List paired device identities without secrets.", objectSchema(map[string]any{}), true}, {"revoke_device", "Revoke device", "Revoke one paired device credential.", objectSchema(map[string]any{"credentialId": str("Credential ID.")}, "credentialId"), true}, {"rename_device", "Rename device", "Rename one paired device identity.", objectSchema(map[string]any{"credentialId": str("Credential ID."), "deviceName": str("New device name.")}, "credentialId", "deviceName"), true}, {"list_workspaces", "List authorized workspaces", "List previously granted workspaces; sleeping workspaces can be activated lazily.", objectSchema(map[string]any{}), true}, {"select_workspace", "Select workspace", "Select and activate a workspace for this MCP session.", objectSchema(map[string]any{"key": str("Workspace key returned by list_workspaces.")}, "key"), true}, {"workspace_info", "Workspace info", "Show the selected or explicitly named workspace.", objectSchema(map[string]any{"workspaceKey": workspaceKeySchema}), true}}
	tools := append(local, remote...)
	return append(tools, automationToolDefinitions()...)
}

func semanticSchema() json.RawMessage {
	return objectSchema(map[string]any{"path": str("Source file path."), "line": integer("1-based line.", 1, 0), "column": integer("1-based column.", 1, 0), "name": str("Symbol name."), "query": str("Fallback query."), "limit": integer("Maximum results.", 1, 2000), "workspaceKey": workspaceKeySchema})
}
func processSchema(approval map[string]any, withYield bool) json.RawMessage {
	p := map[string]any{"command": str("Shell command."), "cwd": str("Workspace-relative working directory."), "timeoutMs": integer("Timeout milliseconds; 0 disables timeout.", 0, 3600000), "approvalToken": approval, "workspaceKey": workspaceKeySchema}
	if withYield {
		p["yieldMs"] = integer("Milliseconds to wait before returning initial output.", 0, 10000)
	}
	return objectSchema(p, "command")
}
func pollSchema() json.RawMessage {
	return objectSchema(map[string]any{"processId": str("Process ID."), "stdoutCursor": integer("Stdout byte cursor.", 0, 0), "stderrCursor": integer("Stderr byte cursor.", 0, 0), "workspaceKey": workspaceKeySchema}, "processId")
}
func writeProcessSchema() json.RawMessage {
	return objectSchema(map[string]any{"processId": str("Process ID."), "input": str("Input bytes as UTF-8 text."), "workspaceKey": workspaceKeySchema}, "processId", "input")
}
func signalSchema() json.RawMessage {
	return objectSchema(map[string]any{"processId": str("Process ID."), "signal": map[string]any{"type": "string", "enum": []string{"SIGTERM", "SIGINT", "SIGKILL"}}, "workspaceKey": workspaceKeySchema}, "processId")
}

const modernMCPProtocolVersion = "2026-07-28"

func isModernMCPProtocolVersion(version string) bool {
	version = strings.TrimSpace(version)
	return len(version) == len(modernMCPProtocolVersion) && version >= modernMCPProtocolVersion
}

func legacyMCPCompatibility(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.TrimSpace(r.Header.Get("Mcp-Session-Id")) == "" {
			modernProbe := isModernMCPProtocolVersion(r.Header.Get("Mcp-Protocol-Version"))
			if !modernProbe && r.ContentLength > 0 && r.ContentLength <= 64<<10 {
				raw, err := io.ReadAll(r.Body)
				if err == nil {
					_ = r.Body.Close()
					r.Body = io.NopCloser(bytes.NewReader(raw))
					var envelope struct {
						Method string `json:"method"`
					}
					if json.Unmarshal(raw, &envelope) == nil && envelope.Method == "server/discover" {
						modernProbe = true
					}
				}
			}
			if modernProbe {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"Invalid or missing MCP session."},"id":null}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) Handler() http.Handler {
	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		claims, ok := oauth.ClaimsFrom(r.Context())
		if !ok || claims.Subject == "" {
			return nil
		}
		return s.serverFor(claims.Subject)
	}, &mcp.StreamableHTTPOptions{Stateless: false, JSONResponse: true, MaxRequestBodyBytes: 4 << 20})
	return legacyMCPCompatibility(stream)
}

func (s *Service) serverFor(userID string) *mcp.Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.servers[userID]; existing != nil {
		return existing
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "codelocal", Version: version.Version}, &mcp.ServerOptions{Instructions: orchestrationInstructions})
	for _, def := range toolDefinitions() {
		definition := def
		title := toolDisplayTitle(def.Name, def.Title)
		server.AddTool(&mcp.Tool{Name: def.Name, Title: title, Annotations: toolAnnotations(def), Description: def.Description, InputSchema: def.Schema}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.callTool(ctx, userID, definition, req)
		})
	}
	s.servers[userID] = server
	return server
}

func sessionID(req *mcp.CallToolRequest) string {
	if req != nil && req.Session != nil {
		return req.Session.ID()
	}
	return "stateless"
}
func decodeArgs(req *mcp.CallToolRequest) (map[string]any, error) {
	args := map[string]any{}
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return args, nil
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	return args, nil
}
func textResultWithNotice(value any, isError bool, notice string) *mcp.CallToolResult {
	var text string
	if raw, err := json.MarshalIndent(value, "", "  "); err == nil {
		text = string(raw)
	} else {
		text = fmt.Sprint(value)
	}
	if strings.TrimSpace(notice) != "" {
		text = notice + "\n\n" + text
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: isError}
}
func textResult(value any, isError bool) *mcp.CallToolResult {
	return textResultWithNotice(value, isError, "")
}
func errorResult(err error) *mcp.CallToolResult {
	return textResult(map[string]any{"error": err.Error()}, true)
}

func (s *Service) route(userID, session string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routes[userID][session]
}
func (s *Service) setRoute(userID, session, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.routes[userID] == nil {
		s.routes[userID] = map[string]string{}
	}
	s.routes[userID][session] = key
}

func (s *Service) claimUpdate(userID, session, workspaceKey, installedVersion string) string {
	notice := clientupdate.Evaluate(installedVersion, s.Release)
	if notice == nil {
		return ""
	}
	sessionKey := userID + ":" + session
	claimKey := workspaceKey + ":" + notice.Key
	s.mu.Lock()
	if s.shownUpdates[sessionKey] == nil {
		s.shownUpdates[sessionKey] = map[string]struct{}{}
	}
	if _, shown := s.shownUpdates[sessionKey][claimKey]; shown {
		s.mu.Unlock()
		return ""
	}
	s.shownUpdates[sessionKey][claimKey] = struct{}{}
	s.mu.Unlock()
	return clientupdate.Render(*notice)
}

func (s *Service) firstUpdateNotice(userID, session string, workspaces []gateway.WorkspaceView) string {
	for _, workspace := range workspaces {
		if notice := s.claimUpdate(userID, session, workspace.Key, workspace.ClientVersion); notice != "" {
			return notice
		}
	}
	return ""
}

func (s *Service) callTool(ctx context.Context, userID string, def toolDef, req *mcp.CallToolRequest) (response *mcp.CallToolResult, retErr error) {
	args, err := decodeArgs(req)
	if err != nil {
		return errorResult(err), nil
	}
	session := sessionID(req)
	_ = s.Store.TouchUserMCPActive(ctx, userID)
	inputBytes, inputTokens := usagecalc.EstimateTokens(args)
	usageDeviceID := ""
	usageWorkspaceID := ""
	defer func() {
		if response == nil {
			return
		}
		outputBytes, outputTokens := usagecalc.EstimateTokens(response.Content)
		event := cloud.MCPUsageEvent{UserID: userID, SessionID: session, DeviceID: usageDeviceID, WorkspaceID: usageWorkspaceID, Tool: def.Name, InputBytes: inputBytes, OutputBytes: outputBytes, InputTokensEst: inputTokens, OutputTokensEst: outputTokens, CreatedAt: time.Now().UnixMilli()}
		go func() {
			usageCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.Store.RecordMCPUsage(usageCtx, event)
		}()
	}()
	if def.Local {
		return s.callLocal(ctx, userID, session, def.Name, args)
	}
	explicit, _ := args["workspaceKey"].(string)
	delete(args, "workspaceKey")
	key := strings.TrimSpace(explicit)
	if key == "" {
		key = s.route(userID, session)
	}
	if key == "" {
		catalog, catalogErr := s.Workspaces.Catalog(ctx, userID)
		if catalogErr != nil {
			return errorResult(catalogErr), nil
		}
		active := []gateway.WorkspaceView{}
		for _, w := range catalog {
			if w.Status == "active" {
				active = append(active, w)
			}
		}
		if len(active) == 1 {
			key = active[0].Key
		} else if len(active) == 0 {
			return errorResult(errors.New("no active workspace; call list_workspaces then select_workspace")), nil
		} else {
			return errorResult(errors.New("multiple workspaces are active; call list_workspaces then select_workspace, and pass workspaceKey for explicit routing")), nil
		}
	}
	workspace, err := s.Workspaces.Activate(ctx, userID, key)
	if err != nil {
		return errorResult(err), nil
	}
	usageDeviceID = workspace.DeviceID
	usageWorkspaceID = workspace.WorkspaceID
	if err := ensureToolSupported(def, workspace); err != nil {
		return errorResult(err), nil
	}
	requestID := cloud.RandomHex(16)
	result, callErr := s.Hub.Call(ctx, userID, key, session, def.Name, args, protocol.SideEffecting(def.Name), requestID)
	if callErr != nil {
		return errorResult(callErr), nil
	}
	if !result.OK {
		return errorResult(errors.New(firstNonEmpty(result.Error, result.ErrorCode, "tool failed"))), nil
	}
	if terminalExecutionTool(def.Name) {
		s.Store.Audit(cloud.AuditEvent{UserID: userID, Event: "terminal.executed", DeviceID: workspace.DeviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{"requestId": requestID, "tool": def.Name}})
	}
	notice := s.claimUpdate(userID, session, workspace.Key, workspace.ClientVersion)
	return toolResultWithNotice(result.Result, false, notice), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func (s *Service) callLocal(ctx context.Context, userID, session, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	switch tool {
	case "list_devices":
		clients := s.Hub.LocalClients(userID)
		groups := map[string][]map[string]any{}
		for _, c := range clients {
			groups[c.DeviceID] = append(groups[c.DeviceID], map[string]any{"workspaceId": c.WorkspaceID, "workspaceName": c.WorkspaceName, "key": c.Key, "projectRoot": c.ProjectRoot, "protocolVersion": c.ProtocolVersion, "clientVersion": c.ClientVersion, "capabilities": c.Capabilities, "lastSeenAt": c.LastSeenAt()})
		}
		devices := []map[string]any{}
		keys := []string{}
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			devices = append(devices, map[string]any{"deviceId": key, "workspaces": groups[key]})
		}
		notice := ""
		for _, client := range clients {
			if notice = s.claimUpdate(userID, session, client.Key, client.ClientVersion); notice != "" {
				break
			}
		}
		return textResultWithNotice(map[string]any{"devices": devices}, false, notice), nil
	case "list_device_identities":
		devices, err := s.Store.ListDevices(ctx, userID)
		if err != nil {
			return errorResult(err), nil
		}
		for i := range devices {
			devices[i].SecretHash = ""
		}
		return textResult(devices, false), nil
	case "revoke_device":
		id, _ := args["credentialId"].(string)
		if id == "" {
			return errorResult(errors.New("credentialId required")), nil
		}
		ok, err := s.Store.RevokeDevice(ctx, userID, id)
		if err != nil {
			return errorResult(err), nil
		}
		return textResult(map[string]any{"revoked": ok}, false), nil
	case "rename_device":
		id, _ := args["credentialId"].(string)
		name, _ := args["deviceName"].(string)
		ok, err := s.Store.RenameDevice(ctx, userID, id, strings.TrimSpace(name))
		if err != nil {
			return errorResult(err), nil
		}
		return textResult(map[string]any{"renamed": ok}, false), nil
	case "list_workspaces":
		catalog, err := s.Workspaces.Catalog(ctx, userID)
		if err != nil {
			return errorResult(err), nil
		}
		notice := s.firstUpdateNotice(userID, session, catalog)
		return textResultWithNotice(map[string]any{"selectedWorkspace": s.route(userID, session), "workspaces": catalog}, false, notice), nil
	case "select_workspace":
		key, _ := args["key"].(string)
		workspace, err := s.Workspaces.Activate(ctx, userID, key)
		if err != nil {
			return errorResult(err), nil
		}
		s.setRoute(userID, session, key)
		notice := s.claimUpdate(userID, session, workspace.Key, workspace.ClientVersion)
		return textResultWithNotice(map[string]any{"selected": key, "workspaceKey": key, "deviceId": workspace.DeviceID, "workspaceId": workspace.WorkspaceID, "workspaceName": workspace.WorkspaceName, "clientVersion": workspace.ClientVersion, "status": "active"}, false, notice), nil
	case "workspace_info":
		key, _ := args["workspaceKey"].(string)
		if strings.TrimSpace(key) == "" {
			key = s.route(userID, session)
		}
		if key == "" {
			return errorResult(errors.New("no workspace selected")), nil
		}
		workspace, err := s.Workspaces.Activate(ctx, userID, key)
		if err != nil {
			return errorResult(err), nil
		}
		notice := s.claimUpdate(userID, session, workspace.Key, workspace.ClientVersion)
		return textResultWithNotice(workspace, false, notice), nil
	default:
		return errorResult(errors.New("unknown local MCP tool")), nil
	}
}
