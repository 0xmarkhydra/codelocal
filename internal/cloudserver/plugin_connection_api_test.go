package cloudserver

import "testing"

func TestValidatePluginConnectionInputRejectsCloudVisibleSecrets(t *testing.T) {
	cases := []pluginConnectInput{
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://token@example.com/mcp"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "http://example.com/mcp"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://example.com/mcp#secret"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://example.com/mcp", BearerEnv: "TOKEN=value"},
	}
	for _, input := range cases {
		if _, err := validatePluginConnectionInput(input); err == nil {
			t.Fatalf("expected rejection for %#v", input)
		}
	}
}

func TestValidatePluginConnectionInputAllowsLocalCredentialReference(t *testing.T) {
	input, err := validatePluginConnectionInput(pluginConnectInput{
		DeviceID:    " mac-1 ",
		WorkspaceID: " repo ",
		Endpoint:    "https://example.com/mcp",
		BearerEnv:   " GITHUB_TOKEN ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.DeviceID != "mac-1" || input.WorkspaceID != "repo" || input.BearerEnv != "GITHUB_TOKEN" {
		t.Fatalf("input not canonicalized: %#v", input)
	}
}

func TestPluginConfigureResultFromAcceptsJSONNumber(t *testing.T) {
	result := pluginConfigureResultFrom(map[string]any{
		"configured": true,
		"connected":  true,
		"serverName": "plugin-github",
		"toolCount":  float64(17),
	})
	if !result.Configured || !result.Connected || result.ServerName != "plugin-github" || result.ToolCount != 17 {
		t.Fatalf("unexpected configure result: %#v", result)
	}
}
