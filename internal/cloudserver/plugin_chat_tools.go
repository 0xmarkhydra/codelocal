package cloudserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/cloudmcp"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

var dashboardPluginChatTools = []map[string]any{
	{"type": "function", "function": map[string]any{"name": "list_plugin_tools", "description": "List configured plugin connections. Supply plugin and connection to discover tools and their input schemas; connection is cloud or local:<device>. Never infer readiness from a saved tool count.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"plugin": map[string]any{"type": "string"}, "connection": map[string]any{"type": "string"}}}}},
	{"type": "function", "function": map[string]any{"name": "call_plugin_tool", "description": "Request user approval for one exact plugin operation. Does not execute it. Show the approval and wait; do not retry or claim completion. Use an exact connection and arguments matching its inputSchema.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"plugin": map[string]any{"type": "string"}, "connection": map[string]any{"type": "string"}, "tool": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"}}, "required": []string{"plugin", "connection", "tool", "arguments"}}}},
}

func pluginJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }

func selectPluginConnection(connections []cloud.PluginConnection, plugin, selector string) (cloud.PluginConnection, error) {
	var found []cloud.PluginConnection
	for _, c := range connections {
		if c.PluginID != plugin {
			continue
		}
		target := "local:" + c.DeviceID
		if c.DeviceID == cloudConnectionDeviceID {
			target = "cloud"
		}
		if selector != "" && selector != target {
			continue
		}
		found = append(found, c)
	}
	if len(found) != 1 {
		return cloud.PluginConnection{}, errors.New("select one exact plugin connection: cloud or local:<device>")
	}
	return found[0], nil
}

func (s *Server) execDashboardPluginListTools(r *http.Request, user string, args map[string]any) (string, bool) {
	if s.Store == nil {
		return pluginJSON(map[string]any{"error": "plugin_store_unavailable"}), true
	}
	connections, err := s.Store.ListPluginConnections(r.Context(), user)
	if err != nil {
		return pluginJSON(map[string]any{"error": "plugin_connections_unavailable"}), true
	}
	plugin := dashboardPluginArgString(args["plugin"])
	selector := dashboardPluginArgString(args["connection"])
	if plugin == "" {
		items := []pluginConnectionDTO{}
		for _, c := range connections {
			dto, err := pluginConnectionDTOFrom(c)
			if err == nil {
				items = append(items, dto)
			}
		}
		return pluginJSON(map[string]any{"connections": items, "note": "Saved status only. Select a connection to test discovery."}), true
	}
	c, err := selectPluginConnection(connections, plugin, selector)
	if err != nil {
		return pluginJSON(map[string]any{"error": "connection_selection_required", "message": err.Error()}), true
	}
	if c.DeviceID == cloudConnectionDeviceID {
		tools, err := s.discoverCloudPlugin(r.Context(), user, c)
		if err != nil {
			return pluginJSON(map[string]any{"error": "plugin_unavailable", "message": err.Error()}), true
		}
		return pluginJSON(map[string]any{"plugin": plugin, "connection": "cloud", "tools": tools}), true
	}
	if s.Hub == nil {
		return pluginJSON(map[string]any{"error": "runtime_gateway_unavailable"}), true
	}
	result, err := s.Hub.Call(r.Context(), user, c.WorkspaceKey, "dashboard-plugin-discovery", "mcp_search_tools", map[string]any{"server": c.ServerName, "query": "", "limit": 50}, false, cloud.RandomHex(16))
	if err != nil || !result.OK {
		return pluginJSON(map[string]any{"error": "device_unavailable"}), true
	}
	return pluginJSON(map[string]any{"connection": "local:" + c.DeviceID, "catalog": result.Result}), true
}

func (s *Server) discoverCloudPlugin(ctx context.Context, user string, c cloud.PluginConnection) ([]cloudmcp.ToolSummary, error) {
	if s.CloudMCP == nil {
		return nil, errors.New("Cloud MCP is unavailable")
	}
	credential, err := s.Store.PluginCloudCredential(ctx, user, c.PluginID)
	if err != nil {
		return nil, errors.New("Reconnect this plugin to authorize Cloud access")
	}
	cfg, err := s.pluginCloudConfig(ctx, user, c.PluginID, c.Endpoint, credential, false)
	if err != nil {
		return nil, err
	}
	return s.CloudMCP.Discover(ctx, user, cfg)
}

func (s *Server) execDashboardPluginCallTool(r *http.Request, user string, args map[string]any) (string, bool) {
	if s.Store == nil || s.WebAuth == nil {
		return pluginJSON(map[string]any{"error": "plugin_store_unavailable"}), true
	}
	identity, err := s.WebAuth.Identity(r)
	if err != nil || identity == nil || identity.User.ID != user {
		return pluginJSON(map[string]any{"error": "unauthorized"}), true
	}
	plugin := strings.TrimSpace(dashboardPluginArgString(args["plugin"]))
	tool := strings.TrimSpace(dashboardPluginArgString(args["tool"]))
	arguments, ok := args["arguments"].(map[string]any)
	raw, _ := json.Marshal(arguments)
	if plugin == "" || tool == "" || len(tool) > 128 || !ok || len(raw) > 64<<10 {
		return pluginJSON(map[string]any{"error": "invalid_plugin_arguments"}), true
	}
	connections, err := s.Store.ListPluginConnections(r.Context(), user)
	if err != nil {
		return pluginJSON(map[string]any{"error": "plugin_connections_unavailable"}), true
	}
	c, err := selectPluginConnection(connections, plugin, dashboardPluginArgString(args["connection"]))
	if err != nil {
		return pluginJSON(map[string]any{"error": "connection_selection_required", "message": err.Error()}), true
	}
	a, err := s.Store.CreatePluginApproval(r.Context(), cloud.PluginApproval{UserID: user, SessionID: identity.SessionID, PluginID: plugin, DeviceID: c.DeviceID, Version: c.UpdatedAt, Tool: tool, Arguments: arguments, ThreadID: dashboardChatThreadID(r)})
	if err != nil {
		return pluginJSON(map[string]any{"error": "approval_unavailable"}), true
	}
	return pluginJSON(map[string]any{"status": "approval_required", "approvalId": a.ID, "plugin": plugin, "connection": dashboardPluginArgString(args["connection"]), "tool": tool, "arguments": arguments, "expiresAt": a.ExpiresAt}), true
}

func (s *Server) pluginApprovalAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.pluginMutationIdentity(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Approve bool `json:"approve"`
	}
	if webutil.DecodeJSON(r, 1024, &input) != nil {
		webutil.JSON(w, 400, map[string]any{"error": "invalid_approval"})
		return
	}
	a, err := s.Store.TakePluginApproval(r.Context(), identity.User.ID, identity.SessionID, r.PathValue("approvalID"), !input.Approve)
	if err != nil {
		webutil.JSON(w, 409, map[string]any{"error": "approval_unavailable"})
		return
	}
	if !input.Approve {
		s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.tool_denied", Detail: map[string]any{"pluginId": a.PluginID, "tool": a.Tool}})
		webutil.JSON(w, 200, map[string]any{"status": "denied"})
		return
	}
	result, err := s.executeApprovedPlugin(r.Context(), identity, a)
	status := "done"
	if err != nil {
		status = "error"
		result = map[string]any{"error": "plugin_call_failed", "message": err.Error()}
	}
	if a.ThreadID != "" {
		tools, _ := json.Marshal([]dashboardToolCall{{ID: a.ID, Name: "call_plugin_tool", Arguments: pluginJSON(map[string]any{"plugin": a.PluginID, "connection": a.DeviceID, "tool": a.Tool, "arguments": a.Arguments}), Result: pluginJSON(result), Status: status}})
		if saveErr := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: "plugin_" + a.ID, UserID: identity.User.ID, ThreadID: a.ThreadID, Role: "assistant", Content: pluginJSON(result), ToolCalls: tools, CreatedAt: time.Now().UnixMilli()}); saveErr != nil {
			webutil.JSON(w, 200, map[string]any{"status": status, "result": result, "warning": "Result could not be saved to chat; do not repeat the operation."})
			return
		}
	}
	if result["isError"] == true {
		status = "error"
	}
	if completeErr := s.Store.CompletePluginApproval(r.Context(), identity.User.ID, a.ID, status); completeErr != nil {
		result = map[string]any{"error": "approval_record_unavailable", "message": "Operation outcome is unknown; check the provider before trying again."}
		status = "error"
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.tool_call", Detail: map[string]any{"pluginId": a.PluginID, "tool": a.Tool, "target": a.DeviceID, "status": status}})
	webutil.JSON(w, 200, map[string]any{"status": status, "result": result})
}

func (s *Server) executeApprovedPlugin(ctx context.Context, identity *webauth.Identity, a cloud.PluginApproval) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	c, exists, err := s.Store.PluginConnectionByDevice(ctx, identity.User.ID, a.PluginID, a.DeviceID)
	if err != nil || !exists || c.UpdatedAt != a.Version {
		return nil, errors.New("connection changed; request a new approval")
	}
	if a.DeviceID == cloudConnectionDeviceID {
		if s.CloudMCP == nil {
			return nil, errors.New("Cloud MCP is unavailable")
		}
		var result map[string]any
		err := s.Store.WithPluginCloudConnection(ctx, identity.User.ID, a.PluginID, a.Version, func(locked cloud.PluginConnection) error {
			credential, err := s.Store.PluginCloudCredential(ctx, identity.User.ID, a.PluginID)
			if err != nil {
				return errors.New("plugin authorization unavailable")
			}
			cfg, err := s.pluginCloudConfig(ctx, identity.User.ID, a.PluginID, locked.Endpoint, credential, false)
			if err != nil {
				return err
			}
			result, err = s.CloudMCP.CallApproved(ctx, identity.User.ID, cfg, a.Tool, a.Arguments)
			return err
		})
		return result, err
	}
	if s.Hub == nil {
		return nil, errors.New("device is unavailable")
	}
	session := "plugin-approved-" + a.ID
	args := map[string]any{"server": c.ServerName, "tool": a.Tool, "arguments": a.Arguments}
	result, err := s.Hub.Call(ctx, identity.User.ID, c.WorkspaceKey, session, "mcp_call", args, true, cloud.RandomHex(16))
	if err != nil {
		return nil, errors.New("device call interrupted; outcome unknown")
	}
	data, _ := result.Result.(map[string]any)
	if data["status"] == "approval_required" {
		token, _ := data["approvalToken"].(string)
		if token == "" {
			return nil, errors.New("device approval is unavailable")
		}
		args["approvalToken"] = token
		result, err = s.Hub.Call(ctx, identity.User.ID, c.WorkspaceKey, session, "mcp_call", args, true, cloud.RandomHex(16))
		if err != nil {
			return nil, errors.New("device call interrupted; outcome unknown")
		}
	}
	data, _ = result.Result.(map[string]any)
	if !result.OK || data["status"] == "blocked" || data["status"] == "approval_required" {
		return nil, errors.New("device refused the plugin operation")
	}
	return map[string]any{"ok": result.OK, "result": result.Result}, nil
}

func (s *Server) pluginApprovalDetailsAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	a, err := s.Store.PluginApproval(r.Context(), identity.User.ID, identity.SessionID, r.PathValue("approvalID"))
	if err != nil {
		webutil.JSON(w, 404, map[string]string{"error": "approval_unavailable"})
		return
	}
	target := "local:" + a.DeviceID
	if a.DeviceID == cloudConnectionDeviceID {
		target = "cloud"
	}
	w.Header().Set("Cache-Control", "no-store")
	webutil.JSON(w, 200, map[string]any{"id": a.ID, "plugin": a.PluginID, "connection": target, "tool": a.Tool, "arguments": a.Arguments, "expiresAt": a.ExpiresAt})
}
