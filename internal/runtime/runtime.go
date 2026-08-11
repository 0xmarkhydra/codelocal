package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/identity"
	"github.com/0xmarkhydra/codelocal/internal/localclient"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	"github.com/0xmarkhydra/codelocal/internal/security"
	usagecalc "github.com/0xmarkhydra/codelocal/internal/usage"
	"github.com/0xmarkhydra/codelocal/internal/version"
	"github.com/0xmarkhydra/codelocal/internal/workspace"
	"github.com/coder/websocket"
)

type Options struct {
	BaseURL       string
	Credential    identity.Credential
	LongPoll      time.Duration
	IdleWorkspace time.Duration
	OnReady       func()
}

type Runtime struct {
	Options         Options
	Registry        *workspace.Registry
	client          *http.Client
	mu              sync.Mutex
	workers         map[string]*WorkspaceWorker
	stopped         bool
	pollCancel      context.CancelFunc
	syncedSignature string
}

type WorkspaceWorker struct {
	Runtime   *Runtime
	Workspace workspace.Workspace
	Engine    *localclient.Engine
	conn      *websocket.Conn
	writeMu   sync.Mutex
	sideMu    sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	lastUsed  atomic.Int64
	callsMu   sync.Mutex
	calls     map[string]context.CancelFunc
}

type pollResponse struct {
	Activation *cloud.WorkspaceActivation `json:"activation"`
	Revocation *cloud.WorkspaceRevocation `json:"revocation"`
	Now        int64                      `json:"now"`
}

func New(options Options) *Runtime {
	if options.LongPoll <= 0 {
		options.LongPoll = 25 * time.Second
	}
	if options.LongPoll > 30*time.Second {
		options.LongPoll = 30 * time.Second
	}
	if options.IdleWorkspace <= 0 {
		options.IdleWorkspace = 20 * time.Minute
	}
	return &Runtime{Options: options, Registry: workspace.New(), client: &http.Client{Timeout: 45 * time.Second}, workers: map[string]*WorkspaceWorker{}}
}
func normalizeBase(value string) string { return strings.TrimRight(value, "/") }
func wsURL(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/client"
	u.RawQuery = ""
	return u.String()
}
func (r *Runtime) headers(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CodeLocal-Credential-Id", r.Options.Credential.CredentialID)
	req.Header.Set("Authorization", "Device "+r.Options.Credential.CredentialSecret)
}
func (r *Runtime) post(ctx context.Context, path string, input any, output any) error {
	raw, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeBase(r.Options.BaseURL)+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	r.headers(req)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("CodeLocal runtime device authorization was revoked")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CodeLocal Cloud %s failed (%d)", path, resp.StatusCode)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}

func registrySignature(items []workspace.Workspace) string {
	parts := make([]string, 0, len(items))
	for _, w := range items {
		parts = append(parts, w.WorkspaceID+"\x00"+w.WorkspaceName+"\x00"+w.LocalPath)
	}
	sortStrings(parts)
	return strings.Join(parts, "\n")
}
func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func (r *Runtime) SyncRegistry(ctx context.Context, force bool) ([]workspace.Workspace, error) {
	items, err := r.Registry.List()
	if err != nil {
		return nil, err
	}
	signature := registrySignature(items)
	r.mu.Lock()
	for id, worker := range r.workers {
		found := false
		for _, w := range items {
			if w.WorkspaceID == id {
				found = true
				break
			}
		}
		if !found {
			go worker.Stop("workspace authorization removed")
		}
	}
	unchanged := signature == r.syncedSignature
	r.mu.Unlock()
	if !force && unchanged {
		return items, nil
	}
	payload := map[string]any{"clientVersion": version.Version, "workspaces": func() []map[string]any {
		out := make([]map[string]any, 0, len(items))
		for _, w := range items {
			out = append(out, map[string]any{"workspaceId": w.WorkspaceID, "workspaceName": w.WorkspaceName})
		}
		return out
	}()}
	if err := r.post(ctx, "/api/client/workspaces/sync", payload, &map[string]any{}); err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.syncedSignature = signature
	r.mu.Unlock()
	return items, nil
}

func (r *Runtime) Status() map[string]any {
	items, _ := r.Registry.List()
	r.mu.Lock()
	active := []map[string]any{}
	for _, worker := range r.workers {
		active = append(active, map[string]any{"workspaceId": worker.Workspace.WorkspaceID, "workspaceName": worker.Workspace.WorkspaceName, "lastUsedAt": worker.lastUsed.Load()})
	}
	r.mu.Unlock()
	return map[string]any{"authorizedWorkspaces": items, "activeWorkspaces": active}
}

func (r *Runtime) Activate(ctx context.Context, workspaceID string) (*WorkspaceWorker, error) {
	r.mu.Lock()
	if existing := r.workers[workspaceID]; existing != nil {
		existing.lastUsed.Store(time.Now().UnixMilli())
		r.mu.Unlock()
		return existing, nil
	}
	r.mu.Unlock()
	entry, err := r.Registry.Get(workspaceID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("workspace is not authorized on this machine: %s", workspaceID)
	}
	worker, err := newWorkspaceWorker(r, *entry)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if existing := r.workers[workspaceID]; existing != nil {
		r.mu.Unlock()
		worker.Stop("duplicate activation")
		return existing, nil
	}
	r.workers[workspaceID] = worker
	r.mu.Unlock()
	if err := r.Registry.MarkActivated(workspaceID); err != nil {
		slog.Warn("failed to persist activation time", "workspaceId", workspaceID, "error", err)
	}
	if err := worker.Start(ctx); err != nil {
		r.mu.Lock()
		if r.workers[workspaceID] == worker {
			delete(r.workers, workspaceID)
		}
		r.mu.Unlock()
		worker.Stop("activation failed")
		return nil, err
	}
	return worker, nil
}

func newWorkspaceWorker(r *Runtime, w workspace.Workspace) (*WorkspaceWorker, error) {
	key := r.Options.Credential.DeviceID + "::" + w.WorkspaceID
	engine, err := localclient.New(w.LocalPath, w.WorkspaceID, w.WorkspaceName, key, r.Options.Credential.DeviceID)
	if err != nil {
		return nil, err
	}
	worker := &WorkspaceWorker{Runtime: r, Workspace: w, Engine: engine, done: make(chan struct{}), calls: map[string]context.CancelFunc{}}
	worker.lastUsed.Store(time.Now().UnixMilli())
	return worker, nil
}

func (w *WorkspaceWorker) Start(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	headers := http.Header{}
	conn, _, err := websocket.Dial(ctx, wsURL(w.Runtime.Options.BaseURL), &websocket.DialOptions{HTTPHeader: headers, CompressionMode: websocket.CompressionContextTakeover})
	if err != nil {
		return err
	}
	conn.SetReadLimit(32 << 20)
	w.conn = conn
	register := protocol.RegisterMessage{Type: "register", ProtocolVersion: protocol.Version, ClientVersion: version.Version, CredentialID: w.Runtime.Options.Credential.CredentialID, CredentialSecret: w.Runtime.Options.Credential.CredentialSecret, DeviceID: w.Runtime.Options.Credential.DeviceID, DeviceName: w.Runtime.Options.Credential.DeviceName, WorkspaceID: w.Workspace.WorkspaceID, WorkspaceName: w.Workspace.WorkspaceName, ProjectRoot: w.Workspace.LocalPath, Capabilities: protocol.Capabilities{Filesystem: true, Git: true, Shell: w.Engine.ShellEnabled, PTY: w.Engine.ShellEnabled, Sandbox: "policy-only", SemanticProviders: w.Engine.SemanticProviders(), Idempotency: true, Cancellation: true, Approvals: true, ApprovalMemory: true, HostPolicyExecution: true, MCPHub: true, TerminalChatApproval: true, TerminalHistory: true}}
	if err := w.send(ctx, register); err != nil {
		conn.Close(websocket.StatusInternalError, "register failed")
		return err
	}
	registeredCtx, cancelRegister := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := conn.Read(registeredCtx)
	cancelRegister()
	if err != nil {
		return err
	}
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil || response["type"] != "registered" {
		return errors.New("CodeLocal Cloud rejected workspace registration")
	}
	go w.loop(ctx)
	go w.idleLoop(ctx)
	return nil
}

func (w *WorkspaceWorker) send(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.Write(ctx, websocket.MessageText, raw)
}
func (w *WorkspaceWorker) loop(ctx context.Context) {
	defer w.Stop("workspace connection ended")
	for {
		_, data, err := w.conn.Read(ctx)
		if err != nil {
			return
		}
		var envelope struct {
			Type           string         `json:"type"`
			RequestID      string         `json:"requestId"`
			ID             string         `json:"id"`
			SessionID      string         `json:"sessionId"`
			WorkspaceKey   string         `json:"workspaceKey"`
			Tool           string         `json:"tool"`
			Args           map[string]any `json:"args"`
			IdempotencyKey string         `json:"idempotencyKey"`
			Reason         string         `json:"reason"`
			Deadline       int64          `json:"deadline"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			continue
		}
		switch envelope.Type {
		case "ping":
			_ = w.send(ctx, map[string]any{"type": "pong", "protocolVersion": protocol.Version, "ts": time.Now().UnixMilli()})
		case "tool_cancel":
			requestID := first(envelope.RequestID, envelope.ID)
			w.callsMu.Lock()
			cancel := w.calls[requestID]
			w.callsMu.Unlock()
			if cancel != nil {
				cancel()
			}
		case "tool_call":
			go w.handleCall(ctx, envelope)
		}
	}
}
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

const (
	terminalANSIReset   = "\x1b[0m"
	terminalANSIBold    = "\x1b[1m"
	terminalANSIGray    = "\x1b[90m"
	terminalANSICyan    = "\x1b[36m"
	terminalANSIGreen   = "\x1b[32m"
	terminalANSIRed     = "\x1b[31m"
	terminalANSIMagenta = "\x1b[35m"
)

func terminalTraceColor(code, value string) string {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return value
	}
	return code + value + terminalANSIReset
}

func terminalTraceDim(value string) string { return terminalTraceColor(terminalANSIGray, value) }

func compactTerminalText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if maxRunes > 1 && len(runes) > maxRunes {
		return string(runes[:maxRunes-1]) + "…"
	}
	return value
}

func terminalToolContext(w workspace.Workspace) string {
	name := strings.TrimSpace(w.WorkspaceName)
	id := strings.TrimSpace(w.WorkspaceID)
	if name != "" && id != "" && name != id {
		return compactTerminalText(name+" · "+id, 48)
	}
	return compactTerminalText(first(name, id, "workspace"), 48)
}

func terminalToolDetail(args map[string]any) string {
	if value, ok := args["command"].(string); ok && strings.TrimSpace(value) != "" {
		return "  $ " + compactTerminalText(security.RedactCommand(value), 64)
	}
	if value, ok := args["path"].(string); ok && strings.TrimSpace(value) != "" {
		return "  " + compactTerminalText(value, 58)
	}
	if value, ok := args["query"].(string); ok && strings.TrimSpace(value) != "" {
		return "  “" + compactTerminalText(value, 58) + "”"
	}
	if value, ok := args["name"].(string); ok && strings.TrimSpace(value) != "" {
		return "  " + compactTerminalText(value, 58)
	}
	if value, ok := args["processId"].(string); ok && strings.TrimSpace(value) != "" {
		return "  process " + compactTerminalText(value, 8)
	}
	return ""
}

func terminalTimestamp(value time.Time) string {
	return value.Format("01/02/2006 - 15:04")
}

func (w *WorkspaceWorker) handleCall(parent context.Context, msg struct {
	Type           string         `json:"type"`
	RequestID      string         `json:"requestId"`
	ID             string         `json:"id"`
	SessionID      string         `json:"sessionId"`
	WorkspaceKey   string         `json:"workspaceKey"`
	Tool           string         `json:"tool"`
	Args           map[string]any `json:"args"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Reason         string         `json:"reason"`
	Deadline       int64          `json:"deadline"`
}) {
	requestID := first(msg.RequestID, msg.ID)
	if requestID == "" {
		return
	}
	ctx := parent
	var cancel context.CancelFunc
	if msg.Deadline > 0 {
		ctx, cancel = context.WithDeadline(parent, time.UnixMilli(msg.Deadline))
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	w.callsMu.Lock()
	if old := w.calls[requestID]; old != nil {
		old()
	}
	w.calls[requestID] = cancel
	w.callsMu.Unlock()
	defer func() { cancel(); w.callsMu.Lock(); delete(w.calls, requestID); w.callsMu.Unlock() }()
	w.lastUsed.Store(time.Now().UnixMilli())
	side := protocol.SideEffecting(msg.Tool)
	if side {
		w.sideMu.Lock()
		defer w.sideMu.Unlock()
	}
	inputBytes, inputTokens := usagecalc.EstimateTokens(msg.Args)
	contextLabel := terminalToolContext(w.Workspace)
	detail := terminalToolDetail(msg.Args)
	if detail != "" {
		detail = terminalTraceDim(detail)
	}
	fmt.Printf("  %s %s %s %s%s\n",
		terminalTraceColor(terminalANSIMagenta, "◆"),
		terminalTraceDim(contextLabel),
		terminalTraceDim("›"),
		terminalTraceColor(terminalANSICyan+terminalANSIBold, msg.Tool),
		detail,
	)
	slog.Debug("MCP tool received", "requestId", requestID, "sessionId", msg.SessionID, "workspace", w.Workspace.WorkspaceID, "tool", msg.Tool)
	startedAt := time.Now()
	result, err := w.Engine.Handle(ctx, msg.Tool, msg.Args, localclient.HandleOptions{RequestID: requestID, SessionID: msg.SessionID, IdempotencyKey: msg.IdempotencyKey})
	response := protocol.ToolResult{Type: "tool_result", ProtocolVersion: protocol.Version, RequestID: requestID, OK: err == nil, Result: result}
	if err != nil {
		response.ErrorCode = normalizeErrorCode(err)
		response.ErrorMessage = err.Error()
	}
	outputBytes, outputTokens := usagecalc.EstimateTokens(response)
	durationMs := time.Since(startedAt).Milliseconds()
	totalTokens := inputTokens + outputTokens
	stamp := terminalTimestamp(time.Now())
	usage := fmt.Sprintf(" - %d token", totalTokens)
	if err == nil {
		fmt.Printf("  %s %s %s %s  %s - %s%s\n",
			terminalTraceColor(terminalANSIGreen, "✓"),
			terminalTraceDim(contextLabel),
			terminalTraceDim("›"),
			terminalTraceColor(terminalANSICyan+terminalANSIBold, msg.Tool),
			terminalTraceDim(fmt.Sprintf("%dms", durationMs)),
			terminalTraceDim(stamp),
			terminalTraceDim(usage),
		)
	} else {
		fmt.Printf("  %s %s %s %s  %s - %s%s  %s\n",
			terminalTraceColor(terminalANSIRed, "✕"),
			terminalTraceDim(contextLabel),
			terminalTraceDim("›"),
			terminalTraceColor(terminalANSICyan+terminalANSIBold, msg.Tool),
			terminalTraceDim(fmt.Sprintf("%dms", durationMs)),
			terminalTraceDim(stamp),
			terminalTraceDim(usage),
			terminalTraceColor(terminalANSIRed, compactTerminalText(err.Error(), 72)),
		)
	}
	slog.Debug("MCP tool completed", "requestId", requestID, "sessionId", msg.SessionID, "workspace", w.Workspace.WorkspaceID, "tool", msg.Tool, "inputBytes", inputBytes, "outputBytes", outputBytes, "inputTokensEstimated", inputTokens, "outputTokensEstimated", outputTokens, "totalTokensEstimated", totalTokens, "durationMs", durationMs)
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = w.send(sendCtx, response)
	sendCancel()
}
func normalizeErrorCode(err error) string {
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "sensitive"):
		return "SENSITIVE_PATH"
	case strings.Contains(lower, "escape") || strings.Contains(lower, "unsafe patch path"):
		return "PATH_ESCAPE"
	case strings.Contains(lower, "changed since read") || strings.Contains(lower, "hash mismatch") || strings.Contains(lower, "conflict"):
		return "CONFLICT"
	case strings.Contains(lower, "duplicate") && strings.Contains(lower, "request"):
		return "DUPLICATE_REQUEST"
	case strings.Contains(lower, "approval") && (strings.Contains(lower, "denied") || strings.Contains(lower, "rejected")):
		return "APPROVAL_DENIED"
	case strings.Contains(lower, "blocked") || strings.Contains(lower, "policy"):
		return "POLICY_BLOCKED"
	case strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return "TOOL_TIMEOUT"
	case strings.Contains(lower, "cancel"):
		return "TOOL_CANCELLED"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "enoent") || strings.Contains(lower, "unknown process"):
		return "NOT_FOUND"
	case strings.Contains(lower, "unsupported") || strings.Contains(lower, "not available"):
		return "UNSUPPORTED"
	default:
		return "TOOL_FAILED"
	}
}
func (w *WorkspaceWorker) idleLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(time.UnixMilli(w.lastUsed.Load())) >= w.Runtime.Options.IdleWorkspace {
				w.Stop("workspace idle")
				return
			}
		}
	}
}
func (w *WorkspaceWorker) Stop(reason string) {
	if w.cancel != nil {
		w.cancel()
	}
	w.callsMu.Lock()
	for _, cancel := range w.calls {
		cancel()
	}
	w.calls = map[string]context.CancelFunc{}
	w.callsMu.Unlock()
	if w.conn != nil {
		_ = w.conn.Close(websocket.StatusNormalClosure, reason)
	}
	if w.Engine != nil {
		w.Engine.Close()
	}
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	w.Runtime.mu.Lock()
	if w.Runtime.workers[w.Workspace.WorkspaceID] == w {
		delete(w.Runtime.workers, w.Workspace.WorkspaceID)
	}
	w.Runtime.mu.Unlock()
}

func (r *Runtime) poll(ctx context.Context) (pollResponse, error) {
	waitMs := r.Options.LongPoll.Milliseconds()
	var response pollResponse
	pollCtx, cancel := context.WithTimeout(ctx, r.Options.LongPoll+10*time.Second)
	r.mu.Lock()
	r.pollCancel = cancel
	r.mu.Unlock()
	defer func() { cancel(); r.mu.Lock(); r.pollCancel = nil; r.mu.Unlock() }()
	items, _ := r.Registry.List()
	ids := make([]string, 0, len(items))
	for _, w := range items {
		ids = append(ids, w.WorkspaceID)
	}
	err := r.post(pollCtx, "/api/client/runtime/poll", map[string]any{"workspaceIds": ids, "waitMs": waitMs}, &response)
	return response, err
}
func (r *Runtime) ackRevocation(ctx context.Context, requestID, workspaceID string) {
	_ = r.post(ctx, "/api/client/runtime/revocation-ack", map[string]any{"requestId": requestID, "workspaceId": workspaceID}, &map[string]any{})
}

func (r *Runtime) Run(ctx context.Context) error {
	items, err := r.SyncRegistry(ctx, true)
	if err != nil {
		return err
	}
	slog.Info("CodeLocal runtime connected", "device", r.Options.Credential.DeviceName, "workspaces", len(items))
	if r.Options.OnReady != nil {
		r.Options.OnReady()
	}
	for {
		r.mu.Lock()
		stopped := r.stopped
		r.mu.Unlock()
		if stopped {
			return nil
		}
		if _, err := r.SyncRegistry(ctx, false); err != nil {
			slog.Warn("workspace sync failed", "error", err)
		}
		message, err := r.poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.mu.Lock()
			stopped = r.stopped
			r.mu.Unlock()
			if stopped {
				return nil
			}
			slog.Warn("runtime long-poll failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if message.Revocation != nil {
			rev := message.Revocation
			removed, revokeErr := r.Registry.Revoke(rev.WorkspaceID)
			if revokeErr != nil {
				slog.Warn("workspace revoke failed", "error", revokeErr)
			}
			r.mu.Lock()
			worker := r.workers[rev.WorkspaceID]
			r.mu.Unlock()
			if worker != nil {
				worker.Stop("workspace revoked")
			}
			if removed {
				_, _ = r.SyncRegistry(ctx, true)
			}
			r.ackRevocation(ctx, rev.RequestID, rev.WorkspaceID)
			continue
		}
		if message.Activation != nil {
			if _, err := r.Activate(ctx, message.Activation.WorkspaceID); err != nil {
				slog.Warn("workspace activation failed", "workspaceId", message.Activation.WorkspaceID, "error", err)
			}
		}
	}
}

func (r *Runtime) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	if r.pollCancel != nil {
		r.pollCancel()
	}
	workers := make([]*WorkspaceWorker, 0, len(r.workers))
	for _, worker := range r.workers {
		workers = append(workers, worker)
	}
	r.mu.Unlock()
	for _, worker := range workers {
		worker.Stop("runtime stopped")
	}
}
