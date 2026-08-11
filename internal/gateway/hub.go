package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	"github.com/coder/websocket"
)

type Client struct {
	Key             string
	UserID          string
	DeviceID        string
	DeviceName      string
	WorkspaceID     string
	WorkspaceName   string
	ProjectRoot     string
	CredentialID    string
	ProtocolVersion int
	ClientVersion   string
	Capabilities    protocol.Capabilities
	ConnectedAt     int64
	lastSeenAt      atomic.Int64
	conn            *websocket.Conn
	writeMu         sync.Mutex
	closed          chan struct{}
}

type localPending struct {
	clientKey string
	result    chan protocol.ToolResult
	startedAt int64
}

type Hub struct {
	Store       *cloud.Store
	Coordinator *Coordinator
	InstanceID  string

	mu        sync.RWMutex
	clients   map[string]*Client
	pending   map[string]localPending
	closed    chan struct{}
	closeOnce sync.Once
}

func ClientKey(userID, deviceID, workspaceID string) string {
	return userID + "::" + deviceID + "::" + workspaceID
}

func equalSecret(actual, expected string) bool {
	if actual == "" || expected == "" || len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func NewHub(store *cloud.Store, instanceID string) *Hub {
	return &Hub{
		Store:      store,
		InstanceID: instanceID,
		clients:    map[string]*Client{},
		pending:    map[string]localPending{},
		closed:     make(chan struct{}),
	}
}

func (h *Hub) SetCoordinator(c *Coordinator) { h.Coordinator = c }

func (h *Hub) localClient(key string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	client := h.clients[key]
	if client == nil {
		return nil
	}
	select {
	case <-client.closed:
		return nil
	default:
		return client
	}
}

func (h *Hub) LocalClients(userID string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := []*Client{}
	for _, client := range h.clients {
		if userID == "" || client.UserID == userID {
			out = append(out, client)
		}
	}
	return out
}

func (h *Hub) LocalClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (c *Client) LastSeenAt() int64 { return c.lastSeenAt.Load() }

func automationCapabilityMap(capabilities protocol.AutomationCapabilities) map[string]any {
	return map[string]any{
		"browser": map[string]any{
			"available":       capabilities.Browser.Available,
			"isolatedProfile": capabilities.Browser.IsolatedProfile,
			"screenshots":     capabilities.Browser.Screenshots,
			"devtools":        capabilities.Browser.Devtools,
			"attachExisting":  capabilities.Browser.AttachExisting,
		},
		"computer": map[string]any{
			"available":         capabilities.Computer.Available,
			"backend":           capabilities.Computer.Backend,
			"windowList":        capabilities.Computer.WindowList,
			"screenCapture":     capabilities.Computer.ScreenCapture,
			"uiTree":            capabilities.Computer.UITree,
			"semanticActions":   capabilities.Computer.SemanticActions,
			"pointer":           capabilities.Computer.Pointer,
			"keyboard":          capabilities.Computer.Keyboard,
			"clipboard":         capabilities.Computer.Clipboard,
			"backgroundControl": capabilities.Computer.BackgroundControl,
			"secureDesktop":     false,
		},
	}
}

func clientCapabilityMap(client *Client) map[string]any {
	capabilities := map[string]any{
		"filesystem":           client.Capabilities.Filesystem,
		"git":                  client.Capabilities.Git,
		"shell":                client.Capabilities.Shell,
		"pty":                  client.Capabilities.PTY,
		"sandbox":              client.Capabilities.Sandbox,
		"semanticProviders":    client.Capabilities.SemanticProviders,
		"idempotency":          client.Capabilities.Idempotency,
		"cancellation":         client.Capabilities.Cancellation,
		"approvals":            client.Capabilities.Approvals,
		"approvalMemory":       client.Capabilities.ApprovalMemory,
		"hostPolicyExecution":  client.Capabilities.HostPolicyExecution,
		"mcpHub":               client.Capabilities.MCPHub,
		"terminalChatApproval": client.Capabilities.TerminalChatApproval,
		"terminalHistory":      client.Capabilities.TerminalHistory,
		"automation":           automationCapabilityMap(client.Capabilities.Automation),
	}
	if client.ClientVersion != "" {
		capabilities["clientVersion"] = client.ClientVersion
	}
	return capabilities
}

func (c *Client) Send(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Write(ctx, websocket.MessageText, payload)
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(16 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer conn.Close(websocket.StatusNormalClosure, "closed")

	authCtx, authCancel := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := conn.Read(authCtx)
	authCancel()
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "registration timeout")
		return
	}

	var msg protocol.RegisterMessage
	if json.Unmarshal(raw, &msg) != nil || msg.Type != "register" {
		_ = conn.Close(websocket.StatusPolicyViolation, "register first")
		return
	}
	// Protocol version was optional before protocol-v2. Match the Node SaaS
	// server and interpret an omitted/zero version as legacy v1.
	if msg.ProtocolVersion == 0 {
		msg.ProtocolVersion = 1
	}
	if !protocol.Compatible(msg.ProtocolVersion) {
		_ = conn.Close(websocket.StatusCode(4406), fmt.Sprintf("CLIENT_UPGRADE_REQUIRED:%d-%d", protocol.MinVersion, protocol.Version))
		return
	}
	// Preserve the previous SaaS behavior for pre-pairing clients. A shared
	// DEVICE_TOKEN cannot identify a tenant, so it must never be upgraded into
	// a multi-tenant credential implicitly. Return the same actionable failure
	// instead of a generic AUTH_FAILED when legacy mode is explicitly enabled.
	if msg.CredentialID == "" && msg.CredentialSecret == "" && msg.Token != "" && os.Getenv("ALLOW_LEGACY_DEVICE_TOKEN") == "1" && equalSecret(msg.Token, os.Getenv("DEVICE_TOKEN")) {
		_ = conn.Close(websocket.StatusCode(4403), "Legacy DEVICE_TOKEN cannot establish a multi-tenant SaaS identity. Pair this device through the website.")
		return
	}

	secretHash := cloud.HashSecret(msg.CredentialSecret)
	device, err := h.Store.AuthenticateDevice(ctx, msg.CredentialID, secretHash)
	if err != nil || device == nil || device.UserID == "" {
		_ = conn.Close(websocket.StatusCode(4403), "AUTH_FAILED")
		return
	}
	if msg.DeviceID == "" {
		msg.DeviceID = device.DeviceID
	}
	if msg.DeviceID != device.DeviceID {
		_ = conn.Close(websocket.StatusCode(4403), "DEVICE_ID_MISMATCH")
		return
	}
	if msg.WorkspaceID == "" {
		_ = conn.Close(websocket.StatusPolicyViolation, "workspaceId required")
		return
	}

	key := ClientKey(device.UserID, msg.DeviceID, msg.WorkspaceID)
	now := time.Now().UnixMilli()
	client := &Client{
		Key:             key,
		UserID:          device.UserID,
		DeviceID:        msg.DeviceID,
		DeviceName:      firstNonEmpty(msg.DeviceName, device.DeviceName, msg.DeviceID),
		WorkspaceID:     msg.WorkspaceID,
		WorkspaceName:   firstNonEmpty(msg.WorkspaceName, msg.WorkspaceID),
		ProjectRoot:     msg.ProjectRoot,
		CredentialID:    device.CredentialID,
		ProtocolVersion: msg.ProtocolVersion,
		ClientVersion:   msg.ClientVersion,
		Capabilities:    msg.Capabilities,
		ConnectedAt:     now,
		conn:            conn,
		closed:          make(chan struct{}),
	}
	client.lastSeenAt.Store(now)

	if err := h.register(ctx, client); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "registration failed")
		return
	}
	defer h.unregister(context.Background(), client)

	_ = client.Send(ctx, map[string]any{
		"type":            "registered",
		"protocolVersion": protocol.Version,
		"serverCapabilities": map[string]any{
			"cancellation":             true,
			"idempotency":              true,
			"pairing":                  true,
			"multiWorkspace":           true,
			"lazyWorkspaceActivation":  true,
			"multiTenant":              true,
			"terminalChatApproval":     true,
			"clientUpdateNotices":      true,
			"horizontalGatewayRouting": true,
		},
	})
	h.Store.Audit(cloud.AuditEvent{
		UserID:      client.UserID,
		Event:       "client.connected",
		DeviceID:    client.DeviceID,
		WorkspaceID: client.WorkspaceID,
		Detail: map[string]any{
			"protocolVersion": client.ProtocolVersion,
			"clientVersion":   client.ClientVersion,
			"gateway":         h.InstanceID,
		},
	})
	slog.Info("client authenticated", "clientKey", key, "gateway", h.InstanceID, "protocolVersion", msg.ProtocolVersion, "clientVersion", msg.ClientVersion)

	for {
		messageType, data, readErr := conn.Read(ctx)
		if readErr != nil {
			return
		}
		if messageType != websocket.MessageText && messageType != websocket.MessageBinary {
			continue
		}
		client.lastSeenAt.Store(time.Now().UnixMilli())

		var envelope struct {
			Type         string `json:"type"`
			RequestID    string `json:"requestId"`
			ID           string `json:"id"`
			OK           bool   `json:"ok"`
			Result       any    `json:"result"`
			Metadata     any    `json:"metadata"`
			ErrorCode    string `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
			Error        string `json:"error"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			continue
		}

		switch envelope.Type {
		case "pong":
			if h.Coordinator != nil {
				_ = h.Coordinator.RefreshOwner(ctx, client.Key, 90*time.Second)
			}
		case "tool_result":
			requestID := envelope.RequestID
			if requestID == "" {
				requestID = envelope.ID
			}
			if requestID == "" {
				continue
			}
			h.mu.Lock()
			pending, ok := h.pending[requestID]
			if ok && pending.clientKey == client.Key {
				delete(h.pending, requestID)
			}
			h.mu.Unlock()
			if !ok || pending.clientKey != client.Key {
				continue
			}
			errMessage := envelope.ErrorMessage
			if errMessage == "" {
				errMessage = envelope.Error
			}
			select {
			case pending.result <- protocol.ToolResult{
				Type:            "tool_result",
				ProtocolVersion: protocol.Version,
				RequestID:       requestID,
				OK:              envelope.OK,
				Result:          envelope.Result,
				Metadata:        envelope.Metadata,
				ErrorCode:       envelope.ErrorCode,
				ErrorMessage:    errMessage,
			}:
			default:
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (h *Hub) register(ctx context.Context, client *Client) error {
	h.mu.Lock()
	old := h.clients[client.Key]
	h.clients[client.Key] = client
	h.mu.Unlock()
	if old != nil && old != client {
		_ = old.conn.Close(websocket.StatusCode(4001), "replaced by newer connection")
	}

	caps := clientCapabilityMap(client)
	if err := h.Store.UpsertWorkspace(ctx, cloud.Workspace{
		UserID:          client.UserID,
		DeviceID:        client.DeviceID,
		WorkspaceID:     client.WorkspaceID,
		WorkspaceName:   client.WorkspaceName,
		ProtocolVersion: client.ProtocolVersion,
		Capabilities:    caps,
	}); err != nil {
		return err
	}
	if h.Coordinator != nil {
		if err := h.Coordinator.ClaimOwner(ctx, client.Key, 90*time.Second); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hub) unregister(ctx context.Context, client *Client) {
	h.mu.Lock()
	current := h.clients[client.Key]
	if current == client {
		delete(h.clients, client.Key)
	}
	for id, pending := range h.pending {
		if pending.clientKey != client.Key {
			continue
		}
		delete(h.pending, id)
		select {
		case pending.result <- protocol.ToolResult{
			RequestID:    id,
			OK:           false,
			ErrorCode:    "CLIENT_OFFLINE",
			ErrorMessage: "Client disconnected during tool call.",
		}:
		default:
		}
	}
	select {
	case <-client.closed:
	default:
		close(client.closed)
	}
	h.mu.Unlock()

	if current == client && h.Coordinator != nil {
		h.Coordinator.ReleaseOwner(ctx, client.Key)
	}
	h.Store.Audit(cloud.AuditEvent{
		UserID:      client.UserID,
		Event:       "client.disconnected",
		DeviceID:    client.DeviceID,
		WorkspaceID: client.WorkspaceID,
		Detail:      map[string]any{"gateway": h.InstanceID},
	})
}

func (h *Hub) HandleRouted(ctx context.Context, call RoutedCall) RoutedResult {
	client := h.localClient(call.ClientKey)
	if client == nil {
		return RoutedResult{RequestID: call.RequestID, OK: false, ErrorCode: "CLIENT_OFFLINE", Error: "workspace connection is not owned by this gateway"}
	}
	result, err := h.callLocal(ctx, client, call)
	if err != nil {
		return RoutedResult{RequestID: call.RequestID, OK: false, ErrorCode: "TOOL_FAILED", Error: err.Error()}
	}
	return RoutedResult{
		RequestID: call.RequestID,
		OK:        result.OK,
		Result:    result.Result,
		Metadata:  result.Metadata,
		ErrorCode: result.ErrorCode,
		Error:     result.ErrorMessage,
	}
}

func (h *Hub) callLocal(ctx context.Context, client *Client, call RoutedCall) (protocol.ToolResult, error) {
	if call.RequestID == "" {
		return protocol.ToolResult{}, errors.New("requestId required")
	}
	wait := make(chan protocol.ToolResult, 1)
	h.mu.Lock()
	if _, exists := h.pending[call.RequestID]; exists {
		h.mu.Unlock()
		return protocol.ToolResult{}, errors.New("duplicate requestId")
	}
	h.pending[call.RequestID] = localPending{clientKey: client.Key, result: wait, startedAt: time.Now().UnixMilli()}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, call.RequestID)
		h.mu.Unlock()
	}()

	var message any
	if client.ProtocolVersion <= 1 {
		message = map[string]any{
			"type": "tool_call",
			"id":   call.RequestID,
			"tool": call.Tool,
			"args": call.Args,
		}
	} else {
		message = protocol.ToolCall{
			Type:            "tool_call",
			ProtocolVersion: protocol.Version,
			RequestID:       call.RequestID,
			SessionID:       call.SessionID,
			WorkspaceKey:    client.Key,
			Tool:            call.Tool,
			Args:            call.Args,
			IdempotencyKey:  call.IdempotencyKey,
			Deadline:        call.Deadline,
		}
	}
	if err := client.Send(ctx, message); err != nil {
		return protocol.ToolResult{}, err
	}

	select {
	case result := <-wait:
		return result, nil
	case <-ctx.Done():
		if client.ProtocolVersion >= 2 {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = client.Send(cancelCtx, protocol.ToolCancel{
				Type:            "tool_cancel",
				ProtocolVersion: protocol.Version,
				RequestID:       call.RequestID,
				Reason:          ctx.Err().Error(),
			})
			cancel()
		}
		return protocol.ToolResult{}, ctx.Err()
	case <-client.closed:
		return protocol.ToolResult{}, errors.New("client disconnected during tool call")
	}
}

func (h *Hub) Call(ctx context.Context, userID, clientKey, sessionID, tool string, args map[string]any, sideEffect bool, requestID string) (RoutedResult, error) {
	if requestID == "" {
		requestID = cloud.RandomHex(16)
	}
	deadline := time.Now().Add(180 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	call := RoutedCall{
		Kind:      "tool",
		RequestID: requestID,
		UserID:    userID,
		ClientKey: clientKey,
		SessionID: sessionID,
		Tool:      tool,
		Args:      args,
		Deadline:  deadline.UnixMilli(),
	}
	if sideEffect {
		call.IdempotencyKey = requestID
	}
	// Keep the hot path on this gateway when it already owns the WebSocket.
	// Coordinator routing remains the fallback for cross-replica ownership.
	if h.localClient(clientKey) != nil {
		return h.HandleRouted(ctx, call), nil
	}
	if h.Coordinator == nil {
		return h.HandleRouted(ctx, call), nil
	}
	return h.Coordinator.Call(ctx, call)
}

func (h *Hub) HeartbeatLoop(ctx context.Context, interval, stale time.Duration) {
	if interval <= 0 {
		interval = 20 * time.Second
	}
	if stale <= 0 {
		stale = 70 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastCredentialSweep := time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.mu.RLock()
			all := make([]*Client, 0, len(h.clients))
			for _, client := range h.clients {
				all = append(all, client)
			}
			h.mu.RUnlock()
			revokedThisTick := map[string]struct{}{}

			if lastCredentialSweep.IsZero() || now.Sub(lastCredentialSweep) >= time.Minute {
				credentialSet := map[string]struct{}{}
				for _, client := range all {
					if client.CredentialID != "" {
						credentialSet[client.CredentialID] = struct{}{}
					}
				}
				credentialIDs := make([]string, 0, len(credentialSet))
				for credentialID := range credentialSet {
					credentialIDs = append(credentialIDs, credentialID)
				}
				if len(credentialIDs) > 0 {
					sweepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					active, err := h.Store.ActiveCredentialIDs(sweepCtx, credentialIDs)
					cancel()
					if err != nil {
						slog.Warn("credential validity sweep failed", "error", err, "gateway", h.InstanceID)
					} else {
						for _, client := range all {
							if client.CredentialID != "" && !active[client.CredentialID] {
								revokedThisTick[client.CredentialID] = struct{}{}
								_ = client.conn.Close(websocket.StatusPolicyViolation, "device credential revoked")
							}
						}
						lastCredentialSweep = now
					}
				} else {
					lastCredentialSweep = now
				}
			}

			for _, client := range all {
				if _, revoked := revokedThisTick[client.CredentialID]; revoked {
					continue
				}
				if now.UnixMilli()-client.lastSeenAt.Load() > stale.Milliseconds() {
					_ = client.conn.Close(websocket.StatusGoingAway, "stale client")
					continue
				}
				pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_ = client.Send(pingCtx, map[string]any{"type": "ping", "protocolVersion": protocol.Version, "ts": now.UnixMilli()})
				if h.Coordinator != nil {
					_ = h.Coordinator.RefreshOwner(pingCtx, client.Key, 90*time.Second)
				}
				cancel()
			}
		}
	}
}

func (h *Hub) Close() {
	h.closeOnce.Do(func() {
		close(h.closed)
		h.mu.RLock()
		clients := make([]*Client, 0, len(h.clients))
		for _, client := range h.clients {
			clients = append(clients, client)
		}
		h.mu.RUnlock()
		for _, client := range clients {
			_ = client.conn.Close(websocket.StatusGoingAway, "server shutdown")
		}
	})
}
