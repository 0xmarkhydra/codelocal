package cloudserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

var pluginCredentialRefRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type pluginConnectionDTO struct {
	DeviceID      string `json:"deviceId"`
	WorkspaceKey  string `json:"workspaceKey"`
	ServerName    string `json:"serverName"`
	Endpoint      string `json:"endpoint"`
	CredentialRef string `json:"credentialRef,omitempty"`
	State         string `json:"state"`
	ToolCount     int    `json:"toolCount"`
	LastError     string `json:"lastError,omitempty"`
	ConnectedAt   int64  `json:"connectedAt,omitempty"`
	UpdatedAt     int64  `json:"updatedAt"`
}

type pluginConnectInput struct {
	DeviceID    string `json:"deviceId"`
	WorkspaceID string `json:"workspaceId"`
	Endpoint    string `json:"endpoint"`
	BearerEnv   string `json:"bearerEnv,omitempty"`
}

type pluginConfigureResult struct {
	Configured bool
	Connected  bool
	ServerName string
	ToolCount  int
	Error      string
}

func pluginConnectionDTOFrom(connection cloud.PluginConnection) pluginConnectionDTO {
	return pluginConnectionDTO{
		DeviceID: connection.DeviceID, WorkspaceKey: connection.WorkspaceKey,
		ServerName: connection.ServerName, Endpoint: connection.Endpoint,
		CredentialRef: connection.CredentialRef, State: string(connection.State),
		ToolCount: connection.ToolCount, LastError: connection.LastError,
		ConnectedAt: connection.ConnectedAt, UpdatedAt: connection.UpdatedAt,
	}
}

func validatePluginConnectionInput(input pluginConnectInput) (pluginConnectInput, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.BearerEnv = strings.TrimSpace(input.BearerEnv)
	if input.DeviceID == "" || input.WorkspaceID == "" {
		return pluginConnectInput{}, errors.New("deviceId and workspaceId are required")
	}
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || parsed.Hostname() == "" {
		return pluginConnectInput{}, errors.New("endpoint must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return pluginConnectInput{}, errors.New("endpoint cannot include credentials or a fragment")
	}
	host := strings.Trim(strings.ToLower(parsed.Hostname()), "[]")
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return pluginConnectInput{}, errors.New("endpoint must use HTTPS unless it is localhost")
	}
	input.Endpoint = parsed.String()
	if input.BearerEnv != "" && !pluginCredentialRefRE.MatchString(input.BearerEnv) {
		return pluginConnectInput{}, errors.New("bearerEnv must be an environment variable name")
	}
	return input, nil
}

func pluginCapability(workspace *gateway.WorkspaceView, name string) bool {
	if workspace == nil || workspace.Capabilities == nil {
		return false
	}
	value, _ := workspace.Capabilities[name].(bool)
	return value
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func pluginConfigureResultFrom(value any) pluginConfigureResult {
	root, _ := value.(map[string]any)
	result := pluginConfigureResult{}
	result.Configured, _ = root["configured"].(bool)
	result.Connected, _ = root["connected"].(bool)
	result.ServerName, _ = root["serverName"].(string)
	result.Error, _ = root["error"].(string)
	result.ToolCount = intFromAny(root["toolCount"])
	return result
}

func (s *Server) pluginWorkspaceByKey(ctx context.Context, userID, workspaceKey string) (*gateway.WorkspaceView, error) {
	if s.Workspaces == nil {
		return nil, errors.New("workspace service is unavailable")
	}
	workspace, err := s.Workspaces.Activate(ctx, userID, workspaceKey)
	if err != nil {
		return nil, err
	}
	if !pluginCapability(workspace, "mcpHub") || !pluginCapability(workspace, "pluginConfig") {
		return nil, errors.New("client_upgrade_required")
	}
	return workspace, nil
}

func (s *Server) pluginConnectionWorkspace(ctx context.Context, userID, deviceID, workspaceID string) (*gateway.WorkspaceView, error) {
	if s.Workspaces == nil {
		return nil, errors.New("workspace service is unavailable")
	}
	catalog, err := s.Workspaces.Catalog(ctx, userID)
	if err != nil {
		return nil, err
	}
	for index := range catalog {
		workspace := catalog[index]
		if workspace.DeviceID == deviceID && workspace.WorkspaceID == workspaceID {
			return s.pluginWorkspaceByKey(ctx, userID, workspace.Key)
		}
	}
	return nil, errors.New("workspace is not available for this account")
}

func (s *Server) pluginConnectAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.pluginMutationIdentity(w, r, true)
	if !ok {
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("pluginID"))
	if _, exists := plugindomain.FindBuiltin(pluginID); !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "plugin_not_found"})
		return
	}
	installation, installed, err := s.Store.PluginInstallationByID(r.Context(), identity.User.ID, pluginID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugins_unavailable"})
		return
	}
	if !installed || installation.State != cloud.PluginInstalled {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "plugin_not_installed"})
		return
	}
	var input pluginConnectInput
	if err := webutil.DecodeJSON(r, 32<<10, &input); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plugin_connection", "detail": "Invalid Plugin connection settings."})
		return
	}
	input, err = validatePluginConnectionInput(input)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plugin_connection", "detail": err.Error()})
		return
	}

	workspace, err := s.pluginConnectionWorkspace(r.Context(), identity.User.ID, input.DeviceID, input.WorkspaceID)
	if err != nil {
		if err.Error() == "client_upgrade_required" {
			webutil.JSON(w, http.StatusConflict, map[string]string{"error": "client_upgrade_required"})
			return
		}
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "plugin_workspace_unavailable", "detail": err.Error()})
		return
	}
	if s.Hub == nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_gateway_unavailable"})
		return
	}

	callCtx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	result, err := s.Hub.Call(callCtx, identity.User.ID, workspace.Key, "plugin-config", "plugin_mcp_configure", map[string]any{
		"pluginId":  pluginID,
		"endpoint":  input.Endpoint,
		"bearerEnv": input.BearerEnv,
	}, true, cloud.RandomHex(16))
	if err != nil || !result.OK {
		detail := "Local CodeLocal runtime could not configure this Plugin."
		if err != nil {
			detail = err.Error()
		} else if strings.TrimSpace(result.Error) != "" {
			detail = result.Error
		}
		webutil.JSON(w, http.StatusBadGateway, map[string]string{"error": "plugin_connect_failed", "detail": detail})
		return
	}

	configured := pluginConfigureResultFrom(result.Result)
	if !configured.Configured || strings.TrimSpace(configured.ServerName) == "" {
		webutil.JSON(w, http.StatusBadGateway, map[string]string{"error": "plugin_connect_failed", "detail": "Local runtime returned an incomplete Plugin configuration result."})
		return
	}
	state := cloud.PluginConnectionConfigured
	if configured.Connected {
		state = cloud.PluginConnectionReady
	} else if strings.TrimSpace(configured.Error) != "" {
		state = cloud.PluginConnectionError
	}
	connection, err := s.Store.SetPluginConnection(r.Context(), cloud.PluginConnection{
		UserID: identity.User.ID, PluginID: pluginID, DeviceID: workspace.DeviceID,
		WorkspaceKey: workspace.Key, ServerName: configured.ServerName, Endpoint: input.Endpoint,
		CredentialRef: input.BearerEnv, State: state, ToolCount: configured.ToolCount, LastError: configured.Error,
	})
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_connection_store_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.connected", DeviceID: workspace.DeviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{
		"pluginId": pluginID, "state": connection.State, "toolCount": connection.ToolCount,
	}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "connection": pluginConnectionDTOFrom(connection)})
}

func (s *Server) pluginDisconnectAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.pluginMutationIdentity(w, r, false)
	if !ok {
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("pluginID"))
	deviceID := strings.TrimSpace(r.PathValue("deviceID"))
	if _, exists := plugindomain.FindBuiltin(pluginID); !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "plugin_not_found"})
		return
	}
	connection, exists, err := s.Store.PluginConnectionByDevice(r.Context(), identity.User.ID, pluginID, deviceID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_connections_unavailable"})
		return
	}
	if !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "plugin_connection_not_found"})
		return
	}
	workspace, err := s.pluginWorkspaceByKey(r.Context(), identity.User.ID, connection.WorkspaceKey)
	if err != nil {
		if err.Error() == "client_upgrade_required" {
			webutil.JSON(w, http.StatusConflict, map[string]string{"error": "client_upgrade_required"})
			return
		}
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "plugin_workspace_unavailable", "detail": err.Error()})
		return
	}
	if s.Hub == nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_gateway_unavailable"})
		return
	}
	callCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := s.Hub.Call(callCtx, identity.User.ID, workspace.Key, "plugin-config", "plugin_mcp_remove", map[string]any{"pluginId": pluginID}, true, cloud.RandomHex(16))
	if err != nil || !result.OK {
		detail := "Local CodeLocal runtime could not remove this Plugin connection."
		if err != nil {
			detail = err.Error()
		} else if strings.TrimSpace(result.Error) != "" {
			detail = result.Error
		}
		webutil.JSON(w, http.StatusBadGateway, map[string]string{"error": "plugin_disconnect_failed", "detail": detail})
		return
	}
	removed, err := s.Store.DeletePluginConnection(r.Context(), identity.User.ID, pluginID, deviceID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_connection_store_failed"})
		return
	}
	if removed {
		s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.disconnected", DeviceID: deviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{"pluginId": pluginID}})
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}
