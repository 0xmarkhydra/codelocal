package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
)

func TestValidatePluginCloudConnectionInputRequiresHTTPSAndToken(t *testing.T) {
	cases := []struct {
		name    string
		input   pluginConnectInput
		wantErr string
	}{
		{
			name:    "plain http endpoint",
			input:   pluginConnectInput{Endpoint: "http://example.com/mcp", BearerToken: "token"},
			wantErr: "HTTPS",
		},
		{
			name:    "localhost endpoint",
			input:   pluginConnectInput{Endpoint: "https://localhost/mcp", BearerToken: "token"},
			wantErr: "localhost",
		},
		{
			name:    "private endpoint",
			input:   pluginConnectInput{Endpoint: "https://192.168.1.5/mcp", BearerToken: "token"},
			wantErr: "private addresses",
		},
		{
			name:    "missing token",
			input:   pluginConnectInput{Endpoint: "https://example.com/mcp"},
			wantErr: "token stored encrypted",
		},
		{
			name:    "env reference alone cannot drive a cloud connection",
			input:   pluginConnectInput{Endpoint: "https://example.com/mcp", BearerEnv: "GITHUB_TOKEN"},
			wantErr: "token stored encrypted",
		},
		{
			name:    "device identity is ignored for cloud",
			input:   pluginConnectInput{Endpoint: "https://example.com/mcp", BearerToken: "token"},
			wantErr: "",
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			validated, err := validatePluginCloudConnectionInput(item.input)
			if item.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if validated.ExecutionTarget != string(plugindomain.ExecutionCloud) {
					t.Fatalf("execution target = %q want cloud", validated.ExecutionTarget)
				}
				if validated.DeviceID != "" || validated.WorkspaceID != "" || validated.BearerEnv != "" {
					t.Fatalf("cloud input retained device-local fields: %#v", validated)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), item.wantErr) {
				t.Fatalf("error = %v want substring %q", err, item.wantErr)
			}
		})
	}
}

func TestPluginConnectionDTOFromDerivesExecutionTarget(t *testing.T) {
	cloudConnection, err := pluginConnectionDTOFrom(cloud.PluginConnection{
		PluginID: "penpot", DeviceID: cloudConnectionDeviceID, WorkspaceKey: cloudConnectionWorkspaceKey,
		ServerName: "plugin-penpot", Endpoint: "https://design.codelocal.cloud/mcp/stream", State: cloud.PluginConnectionReady, ToolCount: 5,
	})
	if err != nil {
		t.Fatalf("cloud DTO failed: %v", err)
	}
	if cloudConnection.ExecutionTarget != string(plugindomain.ExecutionCloud) || cloudConnection.Connection != "cloud" {
		t.Fatalf("unexpected cloud DTO: %#v", cloudConnection)
	}

	deviceConnection, err := pluginConnectionDTOFrom(cloud.PluginConnection{
		PluginID: "github", DeviceID: "device-1", WorkspaceKey: "device-1|ws-1",
		ServerName: "plugin-github", Endpoint: "https://example.com/mcp", State: cloud.PluginConnectionReady, ToolCount: 12,
	})
	if err != nil {
		t.Fatalf("device DTO failed: %v", err)
	}
	if deviceConnection.ExecutionTarget != string(plugindomain.ExecutionLocal) {
		t.Fatalf("unexpected device DTO: %#v", deviceConnection)
	}
}

func TestManifestSupportsCloudMatchesBuiltinCatalog(t *testing.T) {
	catalog := plugindomain.BuiltinCatalog()
	if len(catalog) == 0 {
		t.Fatal("builtin catalog is empty")
	}
	for _, entry := range catalog {
		if !manifestSupportsCloud(entry) {
			t.Fatalf("plugin %q does not declare the cloud execution target", entry.Manifest.ID)
		}
	}
}

func TestDashboardChatExposesPluginTools(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range dashboardChatTools {
		names[dashboardChatToolName(tool)] = true
	}
	if !names["list_plugin_tools"] || !names["call_plugin_tool"] {
		t.Fatalf("dashboard chat tools missing plugin tools: %v", names)
	}
	if !dashboardChatToolReadOnly("list_plugin_tools") {
		t.Fatal("list_plugin_tools must be read-only")
	}
	if dashboardChatToolReadOnly("call_plugin_tool") {
		t.Fatal("call_plugin_tool must not be classified read-only")
	}
}
