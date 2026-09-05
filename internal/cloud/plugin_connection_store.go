package cloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type PluginConnectionState string

const (
	PluginConnectionConfigured PluginConnectionState = "configured"
	PluginConnectionReady      PluginConnectionState = "ready"
	PluginConnectionError      PluginConnectionState = "error"
)

type PluginConnection struct {
	UserID        string                `json:"userId"`
	PluginID      string                `json:"pluginId"`
	DeviceID      string                `json:"deviceId"`
	WorkspaceKey  string                `json:"workspaceKey"`
	ServerName    string                `json:"serverName"`
	Endpoint      string                `json:"endpoint"`
	CredentialRef string                `json:"credentialRef,omitempty"`
	State         PluginConnectionState `json:"state"`
	ToolCount     int                   `json:"toolCount"`
	LastError     string                `json:"lastError,omitempty"`
	ConnectedAt   int64                 `json:"connectedAt,omitempty"`
	UpdatedAt     int64                 `json:"updatedAt"`
}

func normalizePluginConnection(connection PluginConnection) (PluginConnection, error) {
	connection.UserID = strings.TrimSpace(connection.UserID)
	connection.PluginID = strings.TrimSpace(connection.PluginID)
	connection.DeviceID = strings.TrimSpace(connection.DeviceID)
	connection.WorkspaceKey = strings.TrimSpace(connection.WorkspaceKey)
	connection.ServerName = strings.TrimSpace(connection.ServerName)
	connection.Endpoint = strings.TrimSpace(connection.Endpoint)
	connection.CredentialRef = strings.TrimSpace(connection.CredentialRef)
	connection.LastError = strings.TrimSpace(connection.LastError)
	if len(connection.LastError) > 500 {
		connection.LastError = connection.LastError[:500]
	}
	if connection.UserID == "" || connection.PluginID == "" || connection.DeviceID == "" || connection.WorkspaceKey == "" || connection.ServerName == "" || connection.Endpoint == "" {
		return PluginConnection{}, fmt.Errorf("plugin connection identity is required")
	}
	if connection.ToolCount < 0 {
		return PluginConnection{}, fmt.Errorf("plugin connection tool count cannot be negative")
	}
	switch connection.State {
	case PluginConnectionConfigured, PluginConnectionReady, PluginConnectionError:
	default:
		return PluginConnection{}, fmt.Errorf("invalid plugin connection state %q", connection.State)
	}
	connection.UpdatedAt = time.Now().UnixMilli()
	if connection.State == PluginConnectionReady && connection.ConnectedAt <= 0 {
		connection.ConnectedAt = connection.UpdatedAt
	}
	return connection, nil
}

func (s *Store) SetPluginConnection(ctx context.Context, connection PluginConnection) (PluginConnection, error) {
	if s == nil || s.DB == nil {
		return PluginConnection{}, fmt.Errorf("cloud store is unavailable")
	}
	normalized, err := normalizePluginConnection(connection)
	if err != nil {
		return PluginConnection{}, err
	}
	_, err = s.DB.Exec(ctx, `
INSERT INTO codelocal_plugin_connections (
  user_id, plugin_id, device_id, workspace_key, server_name, endpoint, credential_ref,
  state, tool_count, last_error, connected_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (user_id, plugin_id, device_id) DO UPDATE SET
  workspace_key=EXCLUDED.workspace_key,
  server_name=EXCLUDED.server_name,
  endpoint=EXCLUDED.endpoint,
  credential_ref=EXCLUDED.credential_ref,
  state=EXCLUDED.state,
  tool_count=EXCLUDED.tool_count,
  last_error=EXCLUDED.last_error,
  connected_at=CASE
    WHEN EXCLUDED.connected_at > 0 THEN EXCLUDED.connected_at
    ELSE codelocal_plugin_connections.connected_at
  END,
  updated_at=EXCLUDED.updated_at`,
		normalized.UserID, normalized.PluginID, normalized.DeviceID, normalized.WorkspaceKey,
		normalized.ServerName, normalized.Endpoint, normalized.CredentialRef, normalized.State,
		normalized.ToolCount, normalized.LastError, normalized.ConnectedAt, normalized.UpdatedAt,
	)
	if err != nil {
		return PluginConnection{}, err
	}
	return normalized, nil
}

func scanPluginConnection(row pgx.Row, userID, pluginID, deviceID string) (PluginConnection, error) {
	item := PluginConnection{UserID: userID, PluginID: pluginID, DeviceID: deviceID}
	err := row.Scan(
		&item.WorkspaceKey, &item.ServerName, &item.Endpoint, &item.CredentialRef,
		&item.State, &item.ToolCount, &item.LastError, &item.ConnectedAt, &item.UpdatedAt,
	)
	return item, err
}

func (s *Store) PluginConnectionByDevice(ctx context.Context, userID, pluginID, deviceID string) (PluginConnection, bool, error) {
	if s == nil || s.DB == nil {
		return PluginConnection{}, false, fmt.Errorf("cloud store is unavailable")
	}
	userID, pluginID, deviceID = strings.TrimSpace(userID), strings.TrimSpace(pluginID), strings.TrimSpace(deviceID)
	item, err := scanPluginConnection(s.DB.QueryRow(ctx, `
SELECT workspace_key, server_name, endpoint, credential_ref, state, tool_count, last_error, connected_at, updated_at
FROM codelocal_plugin_connections
WHERE user_id=$1 AND plugin_id=$2 AND device_id=$3`, userID, pluginID, deviceID), userID, pluginID, deviceID)
	if err == pgx.ErrNoRows {
		return PluginConnection{}, false, nil
	}
	if err != nil {
		return PluginConnection{}, false, err
	}
	return item, true, nil
}

func (s *Store) ListPluginConnections(ctx context.Context, userID string) ([]PluginConnection, error) {
	if s == nil || s.DB == nil {
		return nil, fmt.Errorf("cloud store is unavailable")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("plugin connection user is required")
	}
	rows, err := s.DB.Query(ctx, `
SELECT plugin_id, device_id, workspace_key, server_name, endpoint, credential_ref, state, tool_count, last_error, connected_at, updated_at
FROM codelocal_plugin_connections
WHERE user_id=$1
ORDER BY updated_at DESC, plugin_id, device_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PluginConnection{}
	for rows.Next() {
		item := PluginConnection{UserID: userID}
		if err := rows.Scan(
			&item.PluginID, &item.DeviceID, &item.WorkspaceKey, &item.ServerName, &item.Endpoint,
			&item.CredentialRef, &item.State, &item.ToolCount, &item.LastError, &item.ConnectedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeletePluginConnection(ctx context.Context, userID, pluginID, deviceID string) (bool, error) {
	if s == nil || s.DB == nil {
		return false, fmt.Errorf("cloud store is unavailable")
	}
	tag, err := s.DB.Exec(ctx, `
DELETE FROM codelocal_plugin_connections
WHERE user_id=$1 AND plugin_id=$2 AND device_id=$3`, strings.TrimSpace(userID), strings.TrimSpace(pluginID), strings.TrimSpace(deviceID))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
