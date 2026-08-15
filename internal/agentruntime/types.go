package agentruntime

import (
	"context"
	"errors"
	"strings"
	"time"
)

type TransportTier string
type IsolationLevel string
type Mode string
type SessionStatus string
type EventKind string

type AuthState string

const (
	TransportNativeStructured TransportTier = "native_structured"
	TransportStructuredCLI    TransportTier = "structured_cli"
	TransportPTY              TransportTier = "pty_compatibility"
	TransportAPI              TransportTier = "provider_api"

	IsolationUnknown   IsolationLevel = "unknown"
	IsolationDegraded  IsolationLevel = "degraded"
	IsolationSandboxed IsolationLevel = "sandboxed"
	IsolationMediated  IsolationLevel = "mediated_tools"

	ModeRead   Mode = "read"
	ModeReview Mode = "review"
	ModeMutate Mode = "mutate"

	SessionStarting  SessionStatus = "starting"
	SessionRunning   SessionStatus = "running"
	SessionWaiting   SessionStatus = "waiting_user"
	SessionCompleted SessionStatus = "completed"
	SessionFailed    SessionStatus = "failed"
	SessionCancelled SessionStatus = "cancelled"

	EventSessionStarted    EventKind = "SESSION_STARTED"
	EventStatus            EventKind = "STATUS"
	EventThinkingSummary   EventKind = "THINKING_SUMMARY"
	EventPlanUpdated       EventKind = "PLAN_UPDATED"
	EventMessage           EventKind = "MESSAGE"
	EventFileRead          EventKind = "FILE_READ"
	EventFileChanged       EventKind = "FILE_CHANGED"
	EventToolStarted       EventKind = "TOOL_STARTED"
	EventToolFinished      EventKind = "TOOL_FINISHED"
	EventCommandStarted    EventKind = "COMMAND_STARTED"
	EventCommandFinished   EventKind = "COMMAND_FINISHED"
	EventApprovalRequested EventKind = "APPROVAL_REQUESTED"
	EventUsage             EventKind = "USAGE"
	EventCheckpoint        EventKind = "CHECKPOINT"
	EventError             EventKind = "ERROR"
	EventSessionCancelled  EventKind = "SESSION_CANCELLED"
	EventSessionCompleted  EventKind = "SESSION_COMPLETED"

	AuthUnknown       AuthState = "unknown"
	AuthAuthenticated AuthState = "authenticated"
	AuthRequired      AuthState = "required"
)

var (
	ErrEngineNotFound      = errors.New("agent engine not found")
	ErrEngineAlreadyExists = errors.New("agent engine already registered")
	ErrUnsafeMutation      = errors.New("agent mutation requires mediated tools or validated sandbox isolation")
	ErrAutoRoutingDisabled = errors.New("automatic agent routing is not enabled in this foundation")
	ErrInvalidAgentRequest = errors.New("invalid agent request")
)

type Capabilities struct {
	Interactive      bool           `json:"interactive"`
	NonInteractive   bool           `json:"nonInteractive"`
	StructuredOutput bool           `json:"structuredOutput"`
	Streaming        bool           `json:"streaming"`
	Resume           bool           `json:"resume"`
	Cancel           bool           `json:"cancel"`
	FileEditing      bool           `json:"fileEditing"`
	ShellExecution   bool           `json:"shellExecution"`
	MCP              bool           `json:"mcp"`
	PlanMode         bool           `json:"planMode"`
	ReviewMode       bool           `json:"reviewMode"`
	Transport        TransportTier  `json:"transport"`
	Isolation        IsolationLevel `json:"isolation"`
}

type ProbeResult struct {
	EngineID     string       `json:"engineId"`
	DisplayName  string       `json:"displayName"`
	Installed    bool         `json:"installed"`
	Executable   string       `json:"executable,omitempty"`
	Version      string       `json:"version,omitempty"`
	Auth         AuthState    `json:"auth"`
	Capabilities Capabilities `json:"capabilities"`
	Compatible   bool         `json:"compatible"`
	Reason       string       `json:"reason,omitempty"`
	CheckedAt    int64        `json:"checkedAt"`
}

type ContextPacket struct {
	Fingerprint     string   `json:"fingerprint,omitempty"`
	RuleFingerprint string   `json:"ruleFingerprint,omitempty"`
	MandatoryRules  []string `json:"mandatoryRules,omitempty"`
	RelevantFacts   []string `json:"relevantFacts,omitempty"`
	Verification    []string `json:"verification,omitempty"`
}

type Request struct {
	TaskID       string        `json:"taskId"`
	ProjectID    string        `json:"projectId,omitempty"`
	WorkspaceKey string        `json:"workspaceKey"`
	Objective    string        `json:"objective"`
	TaskKind     string        `json:"taskKind,omitempty"`
	Mode         Mode          `json:"mode"`
	Context      ContextPacket `json:"context"`
}

type Event struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	TaskID    string         `json:"taskId"`
	EngineID  string         `json:"engineId"`
	Kind      EventKind      `json:"kind"`
	Timestamp int64          `json:"timestamp"`
	Summary   string         `json:"summary,omitempty"`
	Path      string         `json:"path,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Result struct {
	SessionID      string         `json:"sessionId"`
	EngineID       string         `json:"engineId"`
	ProviderStatus string         `json:"providerStatus"`
	Summary        string         `json:"summary,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type Session interface {
	ID() string
	EngineID() string
	Status() SessionStatus
	Events() <-chan Event
	Wait(context.Context) (Result, error)
	Cancel(context.Context) error
}

type Adapter interface {
	ID() string
	DisplayName() string
	Probe(context.Context) ProbeResult
	Capabilities() Capabilities
	Start(context.Context, Request) (Session, error)
}

func normalizeRequest(request Request) (Request, error) {
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.WorkspaceKey = strings.TrimSpace(request.WorkspaceKey)
	request.Objective = strings.Join(strings.Fields(request.Objective), " ")
	request.TaskKind = strings.TrimSpace(request.TaskKind)
	if request.Mode == "" {
		request.Mode = ModeReview
	}
	if request.TaskID == "" || request.WorkspaceKey == "" || request.Objective == "" {
		return Request{}, ErrInvalidAgentRequest
	}
	if request.Mode != ModeRead && request.Mode != ModeReview && request.Mode != ModeMutate {
		return Request{}, ErrInvalidAgentRequest
	}
	return request, nil
}

func mutationIsolationAllowed(capabilities Capabilities) bool {
	return capabilities.Isolation == IsolationMediated || capabilities.Isolation == IsolationSandboxed
}

func nowMillis() int64 { return time.Now().UnixMilli() }
