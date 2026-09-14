package cloudserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/cloudmcp"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

var pluginCredentialRefRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Cloud plugin connections do not belong to a device. The store requires a
// non-empty device/workspace identity, so cloud rows use these sentinels; the
// DTO derives the execution target from the device sentinel.
const (
	cloudConnectionDeviceID     = "cloud"
	cloudConnectionWorkspaceKey = "cloud"
)

type pluginConnectionDTO struct {
	DeviceID        string `json:"deviceId"`
	Connection      string `json:"connection"`
	ExecutionTarget string `json:"executionTarget"`
	WorkspaceKey    string `json:"workspaceKey"`
	ServerName      string `json:"serverName"`
	Endpoint        string `json:"endpoint"`
	CredentialRef   string `json:"credentialRef,omitempty"`
	State           string `json:"state"`
	ToolCount       int    `json:"toolCount"`
	LastError       string `json:"lastError,omitempty"`
	ConnectedAt     int64  `json:"connectedAt,omitempty"`
	UpdatedAt       int64  `json:"updatedAt"`
}

type pluginConnectInput struct {
	DeviceID        string `json:"deviceId"`
	WorkspaceID     string `json:"workspaceId"`
	Endpoint        string `json:"endpoint"`
	BearerEnv       string `json:"bearerEnv,omitempty"`
	BearerToken     string `json:"bearerToken,omitempty"`
	ExecutionTarget string `json:"executionTarget,omitempty"`
	AuthKind        string `json:"authKind,omitempty"`
}

type pluginConfigureResult struct {
	Configured bool
	Connected  bool
	ServerName string
	ToolCount  int
	Error      string
}

func manifestSupportsCloud(entry plugindomain.CatalogEntry) bool {
	for _, component := range entry.Manifest.Components {
		var targets []plugindomain.ExecutionTarget
		if component.App != nil {
			targets = component.App.Execution
		}
		if component.AppTemplate != nil {
			targets = component.AppTemplate.Execution
		}
		for _, target := range targets {
			if target == plugindomain.ExecutionCloud {
				return true
			}
		}
	}
	return false
}

func pluginConnectionDTOFrom(connection cloud.PluginConnection) (pluginConnectionDTO, error) {
	target := plugindomain.ExecutionLocal
	if connection.DeviceID == cloudConnectionDeviceID {
		target = plugindomain.ExecutionCloud
	}
	route, err := plugindomain.ResolveExecutionRoute([]plugindomain.ExecutionTarget{target}, target, connection.DeviceID)
	if err != nil {
		return pluginConnectionDTO{}, err
	}
	return pluginConnectionDTO{
		DeviceID: connection.DeviceID, Connection: route.Connection, ExecutionTarget: string(route.Target), WorkspaceKey: connection.WorkspaceKey,
		ServerName: connection.ServerName, Endpoint: connection.Endpoint,
		CredentialRef: connection.CredentialRef, State: string(connection.State),
		ToolCount: connection.ToolCount, LastError: connection.LastError,
		ConnectedAt: connection.ConnectedAt, UpdatedAt: connection.UpdatedAt,
	}, nil
}

func validatePluginConnectionInput(input pluginConnectInput) (pluginConnectInput, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.BearerEnv = strings.TrimSpace(input.BearerEnv)
	input.BearerToken = strings.TrimSpace(input.BearerToken)
	if input.DeviceID == cloudConnectionDeviceID {
		return input, errors.New("reserved device identity")
	}
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
	if input.BearerEnv != "" && input.BearerToken != "" {
		return pluginConnectInput{}, errors.New("provide bearerEnv or bearerToken, not both")
	}
	if strings.ContainsAny(input.BearerToken, "\r\n\x00") || len(input.BearerToken) > 8192 {
		return pluginConnectInput{}, errors.New("bearerToken is invalid")
	}
	return input, nil
}

func validatePluginCloudConnectionInput(input pluginConnectInput) (pluginConnectInput, error) {
	if input.DeviceID != "" || input.WorkspaceID != "" || input.BearerEnv != "" {
		return input, errors.New("cloud connections must not include device fields or environment references")
	}
	endpoint, err := cloudmcp.ValidateEndpoint(input.Endpoint)
	if err != nil {
		return input, err
	}
	input.Endpoint = endpoint
	input.ExecutionTarget = "cloud"
	if input.AuthKind == "" {
		input.AuthKind = "bearer"
	}
	if input.AuthKind != "bearer" && input.AuthKind != "none" {
		return input, errors.New("use the OAuth sign-in flow for OAuth connections")
	}
	if strings.ContainsAny(input.BearerToken, "\r\n\x00") || len(input.BearerToken) > 8192 {
		return input, errors.New("invalid bearer token")
	}
	if input.AuthKind == "none" && input.BearerToken != "" {
		return input, errors.New("unauthenticated connections must not contain a token")
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
	entry, exists := plugindomain.FindBuiltin(pluginID)
	if !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "plugin_not_found"})
		return
	}
	if !entry.DefaultInstalled {
		installation, installed, err := s.Store.PluginInstallationByID(r.Context(), identity.User.ID, pluginID)
		if err != nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugins_unavailable"})
			return
		}
		if !installed || installation.State != cloud.PluginInstalled {
			webutil.JSON(w, http.StatusConflict, map[string]string{"error": "plugin_not_installed"})
			return
		}
	}
	var input pluginConnectInput
	if err := webutil.DecodeJSON(r, 32<<10, &input); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plugin_connection", "detail": "Invalid Plugin connection settings."})
		return
	}
	if pluginID == "penpot" {
		if strings.TrimSpace(input.BearerEnv) != "" {
			webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plugin_connection", "detail": "Penpot MCP keys must be stored encrypted by CodeLocal."})
			return
		}
		if pasted, parseErr := url.Parse(strings.TrimSpace(input.BearerToken)); parseErr == nil && pasted.IsAbs() {
			if token := strings.TrimSpace(pasted.Query().Get("userToken")); token != "" {
				input.BearerToken = token
			}
		}
		input.Endpoint = plugindomain.ManagedPenpotMCPURL
	}
	executionTarget := plugindomain.ExecutionLocal
	switch strings.TrimSpace(strings.ToLower(input.ExecutionTarget)) {
	case "", string(plugindomain.ExecutionLocal):
	case string(plugindomain.ExecutionCloud):
		executionTarget = plugindomain.ExecutionCloud
	default:
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plugin_connection", "detail": "Unknown Plugin execution target."})
		return
	}
	if executionTarget == plugindomain.ExecutionCloud {
		s.pluginCloudConnect(w, r, identity, pluginID, entry, input)
		return
	}
	input, err := validatePluginConnectionInput(input)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plugin_connection", "detail": err.Error()})
		return
	}
	if pluginID == "penpot" {
		if s.Penpot == nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "penpot_integration_unavailable"})
			return
		}
		if input.BearerToken != "" {
			if err := s.Penpot.ValidateOwner(r.Context(), input.BearerToken, identity.User.Email); err != nil {
				// Adapter errors contain only fixed reasons, never upstream bodies or credentials.
				slog.Warn("Penpot connection identity rejected", "reason", err.Error())
				webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_penpot_identity"})
				return
			}
		}
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
	existingConnection, hasExistingConnection, err := s.Store.PluginConnectionByDevice(r.Context(), identity.User.ID, pluginID, workspace.DeviceID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_connections_unavailable"})
		return
	}
	previousManagedRef := ""
	if hasExistingConnection && plugindomain.IsManagedCredentialReference(pluginID, existingConnection.CredentialRef) {
		previousManagedRef = existingConnection.CredentialRef
	}
	credentialRef := input.BearerEnv
	credentialSecret := ""
	if input.BearerToken != "" {
		credentialRef = plugindomain.ManagedCredentialReference(pluginID)
		if err := s.Store.PutRuntimeSecret(r.Context(), identity.User.ID, cloud.RuntimeScopeWorkspace, workspace.DeviceID, workspace.WorkspaceID, credentialRef, input.BearerToken); err != nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_credential_store_failed"})
			return
		}
		credentialSecret = input.BearerToken
	} else if credentialRef == "" {
		if hasExistingConnection {
			credentialRef = existingConnection.CredentialRef
			if plugindomain.IsManagedCredentialReference(pluginID, credentialRef) {
				secrets, secretErr := s.Store.MaterializeRuntimeSecrets(r.Context(), identity.User.ID, workspace.DeviceID, workspace.WorkspaceID)
				if secretErr != nil {
					webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_credential_materialization_failed"})
					return
				}
				credentialSecret = secrets[credentialRef]
			}
		}
	}

	callCtx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	runtimeArgs := map[string]any{
		"pluginId":  pluginID,
		"endpoint":  input.Endpoint,
		"bearerEnv": credentialRef,
	}
	if credentialSecret != "" {
		if pluginID == "penpot" {
			grant, err := s.OAuth.IssuePenpotGrant(r.Context(), identity.User.ID, workspace.DeviceID, workspace.WorkspaceID, credentialSecret)
			if err != nil {
				webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "penpot_auth_unavailable"})
				return
			}
			credentialSecret = grant
		}
		runtimeArgs["credentialSecret"] = credentialSecret
	}
	if previousManagedRef != "" && previousManagedRef != credentialRef {
		runtimeArgs["clearCredentialRef"] = previousManagedRef
	}
	result, err := s.Hub.Call(callCtx, identity.User.ID, workspace.Key, "plugin-config", "plugin_mcp_configure", runtimeArgs, true, cloud.RandomHex(16))
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
		CredentialRef: credentialRef, State: state, ToolCount: configured.ToolCount, LastError: configured.Error,
	})
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_connection_store_failed"})
		return
	}
	if previousManagedRef != "" && previousManagedRef != credentialRef {
		_ = s.Store.DeleteRuntimeSecret(r.Context(), identity.User.ID, cloud.RuntimeScopeWorkspace, workspace.DeviceID, workspace.WorkspaceID, previousManagedRef)
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.connected", DeviceID: workspace.DeviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{
		"pluginId": pluginID, "state": connection.State, "toolCount": connection.ToolCount,
	}})
	dto, err := pluginConnectionDTOFrom(connection)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]string{"error": "plugin_connection_invalid"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "connection": dto})
}

func (s *Server) materializePenpotGrant(ctx context.Context, userID, deviceID, workspaceID string, secrets map[string]string) {
	ref := plugindomain.ManagedCredentialReference("penpot")
	key := secrets[ref]
	if key == "" {
		return
	}
	// Never fall back to sending the native key when the adapter/signing fails.
	delete(secrets, ref)
	if s.Penpot == nil || s.OAuth == nil {
		return
	}
	grant, err := s.OAuth.IssuePenpotGrant(ctx, userID, deviceID, workspaceID, key)
	if err == nil {
		secrets[ref] = grant
	}
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
	if deviceID == cloudConnectionDeviceID {
		s.pluginCloudDisconnect(w, r, identity, pluginID)
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
	runtimeArgs := map[string]any{"pluginId": pluginID}
	if plugindomain.IsManagedCredentialReference(pluginID, connection.CredentialRef) {
		runtimeArgs["credentialRef"] = connection.CredentialRef
	}
	result, err := s.Hub.Call(callCtx, identity.User.ID, workspace.Key, "plugin-config", "plugin_mcp_remove", runtimeArgs, true, cloud.RandomHex(16))
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
		if plugindomain.IsManagedCredentialReference(pluginID, connection.CredentialRef) {
			_ = s.Store.DeleteRuntimeSecret(r.Context(), identity.User.ID, cloud.RuntimeScopeWorkspace, workspace.DeviceID, workspace.WorkspaceID, connection.CredentialRef)
		}
		s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.disconnected", DeviceID: deviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{"pluginId": pluginID}})
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}

func (s *Server) pluginCloudConnect(w http.ResponseWriter, r *http.Request, identity *webauth.Identity, pluginID string, entry plugindomain.CatalogEntry, input pluginConnectInput) {
	if !manifestSupportsCloud(entry) {
		webutil.JSON(w, 400, map[string]string{"error": "cloud_execution_unsupported"})
		return
	}
	input, err := validatePluginCloudConnectionInput(input)
	if err != nil {
		webutil.JSON(w, 400, map[string]string{"error": "invalid_plugin_connection", "detail": err.Error()})
		return
	}
	credential := cloud.PluginCloudCredential{Kind: input.AuthKind, Token: input.BearerToken, Endpoint: input.Endpoint}
	if credential.Kind == "bearer" && credential.Token == "" {
		existing, exists, loadErr := s.Store.PluginConnectionByDevice(r.Context(), identity.User.ID, pluginID, cloudConnectionDeviceID)
		if loadErr != nil || !exists || existing.Endpoint != input.Endpoint {
			webutil.JSON(w, 400, map[string]string{"error": "plugin_token_required"})
			return
		}
		credential, err = s.Store.PluginCloudCredential(r.Context(), identity.User.ID, pluginID)
		if err != nil {
			webutil.JSON(w, 400, map[string]string{"error": "plugin_reconnect_required"})
			return
		}
	}
	cfg, err := s.pluginCloudConfig(r.Context(), identity.User.ID, pluginID, input.Endpoint, credential, true)
	if err != nil {
		webutil.JSON(w, 400, map[string]string{"error": "plugin_auth_unavailable", "detail": err.Error()})
		return
	}
	if s.CloudMCP == nil {
		webutil.JSON(w, 503, map[string]string{"error": "plugin_cloud_runtime_unavailable"})
		return
	}
	tools, err := s.CloudMCP.Discover(r.Context(), identity.User.ID, cfg)
	if err != nil {
		webutil.JSON(w, 502, map[string]string{"error": "plugin_connect_failed", "detail": err.Error()})
		return
	}
	connection, err := s.Store.SavePluginCloudConnection(r.Context(), cloud.PluginConnection{
		UserID: identity.User.ID, PluginID: pluginID, ServerName: "plugin-" + pluginID, Endpoint: input.Endpoint,
		State: cloud.PluginConnectionReady, ToolCount: len(tools),
	}, credential)
	if err != nil {
		webutil.JSON(w, 503, map[string]string{"error": "plugin_connection_store_failed"})
		return
	}
	dto, err := pluginConnectionDTOFrom(connection)
	if err != nil {
		webutil.JSON(w, 500, map[string]string{"error": "plugin_connection_invalid"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.connected", Detail: map[string]any{"pluginId": pluginID, "executionTarget": "cloud", "toolCount": len(tools)}})
	webutil.JSON(w, 200, map[string]any{"ok": true, "connection": dto})
}

func (s *Server) pluginCloudConfig(ctx context.Context, user, plugin, endpoint string, credential cloud.PluginCloudCredential, probe bool) (cloudmcp.Config, error) {
	cfg := cloudmcp.Config{Endpoint: endpoint, Bearer: credential.Token}
	if credential.Endpoint != endpoint {
		return cfg, errors.New("plugin endpoint changed; reconnect required")
	}
	if credential.Kind == "oauth" && credential.ExpiresAt > 0 && credential.ExpiresAt <= time.Now().Add(time.Minute).Unix() {
		fresh, err := s.Store.RefreshPluginCloudCredential(ctx, user, plugin, func(c cloud.PluginCloudCredential) (cloud.PluginCloudCredential, error) {
			if c.Endpoint != endpoint {
				return c, errors.New("plugin endpoint changed")
			}
			if c.RefreshToken == "" {
				return c, errors.New("OAuth session expired; sign in again")
			}
			client, closeClient := cloudmcp.NewMetadataClient()
			defer closeClient()
			token, err := cloudmcp.RefreshOAuth(ctx, client, cloudmcp.OAuthFlow{Endpoint: endpoint, TokenEndpoint: c.TokenEndpoint, ClientID: c.ClientID, Issuer: c.Issuer}, c.RefreshToken)
			if err != nil {
				return c, err
			}
			c.Token = token.AccessToken
			if token.RefreshToken != "" {
				c.RefreshToken = token.RefreshToken
			}
			c.ExpiresAt = 0
			if token.ExpiresIn > 0 {
				c.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).Unix()
			}
			return c, nil
		})
		if err != nil {
			return cfg, errors.New("OAuth refresh failed; sign in again")
		}
		cfg.Bearer = fresh.Token
	}
	if plugin == "penpot" {
		if endpoint != plugindomain.ManagedPenpotMCPURL || s.Penpot == nil || s.OAuth == nil {
			return cfg, errors.New("Penpot integration is unavailable")
		}
		account, err := s.Store.UserByID(ctx, user)
		if err != nil || account == nil {
			return cfg, errors.New("Penpot account unavailable")
		}
		if err := s.Penpot.ValidateOwner(ctx, credential.Token, account.Email); err != nil {
			return cfg, errors.New("Penpot key is invalid or belongs to another account")
		}
		token, err := s.OAuth.IssuePenpotCloudGrant(ctx, user, credential.Token, probe)
		if err != nil {
			return cfg, errors.New("Penpot authorization unavailable")
		}
		cfg.Bearer = token
	}
	return cfg, nil
}

func (s *Server) pluginCloudDisconnect(w http.ResponseWriter, r *http.Request, identity *webauth.Identity, pluginID string) {
	removed, err := s.Store.DeletePluginCloudConnection(r.Context(), identity.User.ID, pluginID)
	if err != nil {
		webutil.JSON(w, 503, map[string]string{"error": "plugin_connection_store_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.disconnected", Detail: map[string]any{"pluginId": pluginID, "executionTarget": "cloud"}})
	webutil.JSON(w, 200, map[string]any{"ok": true, "removed": removed})
}
