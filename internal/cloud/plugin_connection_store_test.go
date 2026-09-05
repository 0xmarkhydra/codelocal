package cloud

import (
	"strings"
	"testing"
)

func TestNormalizePluginConnectionRequiresStableIdentity(t *testing.T) {
	connection, err := normalizePluginConnection(PluginConnection{
		UserID: " user-1 ", PluginID: " github ", DeviceID: " mac-1 ",
		WorkspaceKey: " user-1::mac-1::repo ", ServerName: " plugin-github ",
		Endpoint: " https://example.com/mcp ", CredentialRef: " GITHUB_TOKEN ",
		State: PluginConnectionReady, ToolCount: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if connection.UserID != "user-1" || connection.PluginID != "github" || connection.CredentialRef != "GITHUB_TOKEN" {
		t.Fatalf("connection was not canonicalized: %#v", connection)
	}
	if connection.ConnectedAt <= 0 || connection.UpdatedAt <= 0 {
		t.Fatalf("ready connection needs timestamps: %#v", connection)
	}
}

func TestNormalizePluginConnectionRejectsInvalidStateAndToolCount(t *testing.T) {
	base := PluginConnection{
		UserID: "user-1", PluginID: "github", DeviceID: "mac-1",
		WorkspaceKey: "user-1::mac-1::repo", ServerName: "plugin-github",
		Endpoint: "https://example.com/mcp", State: PluginConnectionReady,
	}
	invalidState := base
	invalidState.State = "unknown"
	if _, err := normalizePluginConnection(invalidState); err == nil {
		t.Fatal("invalid state must be rejected")
	}
	invalidCount := base
	invalidCount.ToolCount = -1
	if _, err := normalizePluginConnection(invalidCount); err == nil {
		t.Fatal("negative tool count must be rejected")
	}
}

func TestNormalizePluginConnectionBoundsStoredError(t *testing.T) {
	connection, err := normalizePluginConnection(PluginConnection{
		UserID: "user-1", PluginID: "github", DeviceID: "mac-1",
		WorkspaceKey: "user-1::mac-1::repo", ServerName: "plugin-github",
		Endpoint: "https://example.com/mcp", State: PluginConnectionError,
		LastError: strings.Repeat("x", 900),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(connection.LastError) != 500 {
		t.Fatalf("stored error length=%d want 500", len(connection.LastError))
	}
}
