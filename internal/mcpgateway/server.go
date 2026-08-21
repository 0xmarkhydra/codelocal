package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/clientupdate"
	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/oauth"
	usagecalc "github.com/0xmarkhydra/codelocal/internal/usage"
	"github.com/0xmarkhydra/codelocal/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Service struct {
	Store        *cloud.Store
	Hub          *gateway.Hub
	Workspaces   *gateway.WorkspaceService
	Memory       longTermMemoryStore
	Release      clientupdate.Manifest
	mu           sync.Mutex
	servers      map[string]*mcp.Server
	routes       map[string]map[string]string
	shownUpdates map[string]map[string]struct{}

	semanticCanaryMu    sync.Mutex
	semanticCanaryGates map[string]semanticCanaryGateEntry
}

func New(store *cloud.Store, hub *gateway.Hub, workspaces *gateway.WorkspaceService, memories ...longTermMemoryStore) *Service {
	var memoryStore longTermMemoryStore
	if len(memories) > 0 {
		memoryStore = memories[0]
	}
	return &Service{
		Store:               store,
		Hub:                 hub,
		Workspaces:          workspaces,
		Memory:              memoryStore,
		Release:             clientupdate.ManifestFromEnv(),
		servers:             map[string]*mcp.Server{},
		routes:              map[string]map[string]string{},
		shownUpdates:        map[string]map[string]struct{}{},
		semanticCanaryGates: map[string]semanticCanaryGateEntry{},
	}
}

func objectSchema(properties map[string]any, required ...string) json.RawMessage {
	value := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		value["required"] = required
	}
	raw, _ := json.Marshal(value)
	return raw
}
func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
func boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}
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

var workspaceKeySchema = map[string]any{"type": "string", "minLength": 1, "description": "Exact workspace key returned by workspace(action=list/select). Pass it to keep routing explicit across multiple active projects, AI client conversations or fresh MCP sessions."}

func boolPtr(value bool) *bool { return &value }
func protocolOneRuntimeTool(name string) bool {
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

func ensureOperationSupported(operation operationInvocation, workspace *gateway.WorkspaceView) error {
	if workspace == nil {
		return errors.New("workspace unavailable")
	}
	if automationRuntimeTool(operation.RuntimeTool) {
		return ensureAutomationOperationSupported(operation.RuntimeTool, workspace)
	}
	if operation.RuntimeTool == "approval_mode" && workspace.ProtocolVersion < 3 {
		return fmt.Errorf("%s requires CodeLocal protocol v3 or newer; update the client before changing chat access mode", operation.OperationID)
	}
	if workspace.ProtocolVersion <= 1 {
		if !protocolOneRuntimeTool(operation.RuntimeTool) {
			return fmt.Errorf("%s requires a newer CodeLocal client; update the client before using this operation", operation.OperationID)
		}
		return nil
	}
	capability := operation.Capability
	if capability == "filesystem" && (operation.RuntimeTool == "sandbox_info" || operation.RuntimeTool == "sandbox_smoke_test") {
		return nil
	}
	if capability == "pty" {
		if !capabilityBool(workspace.Capabilities, "shell") || !capabilityBool(workspace.Capabilities, "pty") {
			return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise PTY support", operation.OperationID)
		}
		return nil
	}
	if !capabilityBool(workspace.Capabilities, capability) {
		return fmt.Errorf("%s is unavailable because this CodeLocal client does not advertise %s support", operation.OperationID, capability)
	}
	return nil
}

const modernMCPProtocolVersion = "2026-07-28"

func isModernMCPProtocolVersion(version string) bool {
	version = strings.TrimSpace(version)
	return len(version) == len(modernMCPProtocolVersion) && version >= modernMCPProtocolVersion
}

func statefulMCPCompatibility(next http.Handler) http.Handler {
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
				// Keep the proven 2025-era stateful handshake used by the previous
				// TypeScript gateway. CodeLocal still relies on MCP session IDs to
				// isolate workspace selection between ChatGPT threads. The Go SDK's
				// 2026-07-28 HTTP era is stateless, so reject the modern probe and let
				// auto-negotiating clients fall back to the stateful initialize/tools/list flow.
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
	surface := PublicToolSurface()
	compatible := statefulMCPCompatibility(stream)
	withSurface := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CodeLocal-Tool-Surface-Version", fmt.Sprint(surface.Version))
		w.Header().Set("X-CodeLocal-Tool-Surface-Hash", surface.Hash)
		compatible.ServeHTTP(w, r)
	})
	return withSurface
}

func (s *Service) ToolSurface() ToolSurfaceInfo { return PublicToolSurface() }

func (s *Service) serverFor(userID string) *mcp.Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.servers[userID]; existing != nil {
		return existing
	}
	instructions := compactOrchestrationInstructions + "\n\nCompatibility: " + toolSurfaceSummary() + ". If CodeLocal reports CODELOCAL_TOOL_SCHEMA_STALE or CODELOCAL_TOOL_SCHEMA_MISMATCH, finish the current request when possible and tell the user to reconnect or refresh CodeLocal in the current AI client so the latest tool schema is loaded."
	server := mcp.NewServer(&mcp.Implementation{Name: "codelocal", Version: version.Version}, &mcp.ServerOptions{Instructions: instructions})
	registerCompactTools(server, s, userID)
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
	structured := map[string]any{}
	if raw, err := json.Marshal(value); err == nil {
		text = string(raw)
		// MCP structuredContent is object-shaped. Runtime operations such as
		// computer_list_windows legitimately return a top-level array, so keep the
		// compatibility text unchanged but wrap non-object JSON values for clients
		// that validate structuredContent strictly.
		var normalized any
		if json.Unmarshal(raw, &normalized) == nil {
			if root, ok := normalized.(map[string]any); ok {
				structured = root
			} else {
				structured["result"] = normalized
			}
		}
	} else {
		text = fmt.Sprint(value)
		structured["result"] = text
	}
	structured["codeLocalToolSurface"] = PublicToolSurface()
	if strings.TrimSpace(notice) != "" {
		text = notice + "\n\n" + text
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, StructuredContent: structured, IsError: isError}
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

func (s *Service) callOperation(ctx context.Context, userID, publicTool string, operation operationInvocation, args map[string]any, req *mcp.CallToolRequest) (response *mcp.CallToolResult, retErr error) {
	startedAt := time.Now()
	session := sessionID(req)
	inputBytes, inputTokens := usagecalc.EstimateTokens(args)
	usageDeviceID := ""
	usageWorkspaceID := ""
	defer func() {
		if response == nil || s.Store == nil {
			return
		}
		outputBytes, outputTokens := usagecalc.EstimateTokens(response.Content)
		event := cloud.MCPUsageEvent{UserID: userID, SessionID: session, DeviceID: usageDeviceID, WorkspaceID: usageWorkspaceID, Tool: publicTool, InputBytes: inputBytes, OutputBytes: outputBytes, InputTokensEst: inputTokens, OutputTokensEst: outputTokens, CreatedAt: time.Now().UnixMilli()}
		// Usage is telemetry, so request latency must never depend on Redis or
		// PostgreSQL. RecordMCPUsage only offers the event to a bounded local
		// queue; background workers publish it durably and persist it in order.
		_ = s.Store.RecordMCPUsage(context.Background(), event)
	}()
	if operation.Local {
		return s.callLocal(ctx, userID, session, operation.RuntimeTool, args)
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
			return errorResult(workspaceRoutingError(false)), nil
		} else {
			return errorResult(workspaceRoutingError(true)), nil
		}
	}
	workspace, err := s.Workspaces.Activate(ctx, userID, key)
	if err != nil {
		return errorResult(err), nil
	}
	usageDeviceID = workspace.DeviceID
	usageWorkspaceID = workspace.WorkspaceID
	if err := ensureOperationSupported(operation, workspace); err != nil {
		compatibility := map[string]any{
			"error":                  err.Error(),
			"code":                   "CODELOCAL_OPERATION_UNSUPPORTED",
			"installedClientVersion": workspace.ClientVersion,
			"protocolVersion":        workspace.ProtocolVersion,
			"toolSurface":            PublicToolSurface(),
		}
		if update := clientupdate.Evaluate(workspace.ClientVersion, s.Release); update != nil {
			compatibility["updateAvailable"] = true
			compatibility["latestClientVersion"] = update.LatestVersion
			compatibility["updateCommand"] = update.UpdateCommand
			compatibility["restartCommand"] = update.RestartCommand
		}
		notice := s.claimUpdate(userID, session, workspace.Key, workspace.ClientVersion)
		return textResultWithNotice(compatibility, true, notice), nil
	}
	requestID := cloud.RandomHex(16)
	result, callErr := s.Hub.Call(ctx, userID, key, session, operation.RuntimeTool, args, operation.SideEffecting, requestID)
	totalDurationMs := time.Since(startedAt).Milliseconds()
	runtimeDurationMs := metadataInt64(result.Metadata, "runtimeDurationMs")
	relayDurationMs := totalDurationMs - runtimeDurationMs
	if relayDurationMs < 0 {
		relayDurationMs = 0
	}
	slog.Debug("MCP gateway operation completed", "requestId", requestID, "publicTool", publicTool, "operationId", operation.OperationID, "runtimeTool", operation.RuntimeTool, "workspace", key, "ok", callErr == nil && result.OK, "durationMs", totalDurationMs, "runtimeDurationMs", runtimeDurationMs, "relayDurationMs", relayDurationMs)
	if callErr != nil {
		return errorResult(callErr), nil
	}
	if !result.OK {
		return errorResult(errors.New(firstNonEmpty(result.Error, result.ErrorCode, "tool failed"))), nil
	}
	if operation.TerminalExecution && s.Store != nil {
		s.Store.Audit(cloud.AuditEvent{UserID: userID, Event: "terminal.executed", DeviceID: workspace.DeviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{"requestId": requestID, "tool": publicTool, "runtimeTool": operation.RuntimeTool, "operationId": operation.OperationID}})
	}
	notice := s.claimUpdate(userID, session, workspace.Key, workspace.ClientVersion)
	return toolResultWithNotice(result.Result, false, notice), nil
}

func workspaceRoutingError(multiple bool) error {
	if multiple {
		return errors.New("multiple workspaces are active; call workspace(action=select), and pass workspaceKey for explicit routing")
	}
	return errors.New("no active workspace; call workspace(action=list) then workspace(action=select)")
}

func metadataInt64(metadata any, key string) int64 {
	values, ok := metadata.(map[string]any)
	if !ok {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	default:
		return 0
	}
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
	case "memory_remember":
		return s.rememberConversationMemory(ctx, userID, session, args)
	case "memory_recall":
		return s.recallConversationMemory(ctx, userID, session, args)
	default:
		return errorResult(errors.New("unknown local MCP tool")), nil
	}
}
