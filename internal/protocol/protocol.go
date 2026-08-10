package protocol

import "time"

const (
	Version    = 2
	MinVersion = 1
)

type Capabilities struct {
	Filesystem           bool     `json:"filesystem"`
	Git                  bool     `json:"git"`
	Shell                bool     `json:"shell"`
	PTY                  bool     `json:"pty"`
	Sandbox              string   `json:"sandbox"`
	SemanticProviders    []string `json:"semanticProviders"`
	Idempotency          bool     `json:"idempotency"`
	Cancellation         bool     `json:"cancellation"`
	Approvals            bool     `json:"approvals"`
	ApprovalMemory       bool     `json:"approvalMemory,omitempty"`
	HostPolicyExecution  bool     `json:"hostPolicyExecution,omitempty"`
	MCPHub               bool     `json:"mcpHub,omitempty"`
	TerminalChatApproval bool     `json:"terminalChatApproval,omitempty"`
	TerminalHistory      bool     `json:"terminalHistory,omitempty"`
}

type RegisterMessage struct {
	Type             string       `json:"type"`
	ProtocolVersion  int          `json:"protocolVersion"`
	ClientVersion    string       `json:"clientVersion,omitempty"`
	Token            string       `json:"token,omitempty"`
	CredentialID     string       `json:"credentialId,omitempty"`
	CredentialSecret string       `json:"credentialSecret,omitempty"`
	DeviceID         string       `json:"deviceId"`
	DeviceName       string       `json:"deviceName,omitempty"`
	WorkspaceID      string       `json:"workspaceId"`
	WorkspaceName    string       `json:"workspaceName"`
	ProjectRoot      string       `json:"projectRoot,omitempty"`
	Capabilities     Capabilities `json:"capabilities"`
}

type ToolCall struct {
	Type            string         `json:"type"`
	ProtocolVersion int            `json:"protocolVersion"`
	RequestID       string         `json:"requestId"`
	SessionID       string         `json:"sessionId,omitempty"`
	WorkspaceKey    string         `json:"workspaceKey"`
	Tool            string         `json:"tool"`
	Args            map[string]any `json:"args,omitempty"`
	IdempotencyKey  string         `json:"idempotencyKey,omitempty"`
	Deadline        int64          `json:"deadline,omitempty"`
}

func (m ToolCall) DeadlineTime() (time.Time, bool) {
	if m.Deadline <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(m.Deadline), true
}

type ToolCancel struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocolVersion"`
	RequestID       string `json:"requestId"`
	Reason          string `json:"reason,omitempty"`
}

type ToolResult struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocolVersion"`
	RequestID       string `json:"requestId"`
	OK              bool   `json:"ok"`
	Result          any    `json:"result,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
	Metadata        any    `json:"metadata,omitempty"`
}

func Compatible(v int) bool { return v >= MinVersion && v <= Version }

func SideEffecting(tool string) bool {
	switch tool {
	case "write_file", "edit_file", "apply_patch", "apply_edits", "format_changed_files",
		"run_command", "exec_start", "pty_start", "process_write", "process_kill", "exec_cancel",
		"git_stage", "git_unstage", "git_commit", "git_push", "approval_revoke", "approval_reset", "mcp_call":
		return true
	default:
		return false
	}
}
