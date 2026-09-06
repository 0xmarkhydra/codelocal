package cloudserver

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
)

func TestValidatePluginConnectionInputRejectsEmbeddedAndAmbiguousSecrets(t *testing.T) {
	cases := []pluginConnectInput{
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://token@example.com/mcp"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "http://example.com/mcp"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://example.com/mcp#secret"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://example.com/mcp", BearerEnv: "TOKEN=value"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://example.com/mcp", BearerEnv: "TOKEN", BearerToken: "secret"},
		{DeviceID: "d", WorkspaceID: "w", Endpoint: "https://example.com/mcp", BearerToken: "secret\nvalue"},
	}
	for _, input := range cases {
		if _, err := validatePluginConnectionInput(input); err == nil {
			t.Fatalf("expected rejection for %#v", input)
		}
	}
}

func TestValidatePluginConnectionInputAcceptsServerEncryptedBearerToken(t *testing.T) {
	input, err := validatePluginConnectionInput(pluginConnectInput{
		DeviceID: "mac-1", WorkspaceID: "repo", Endpoint: "https://example.com/mcp", BearerToken: " secret-value ",
	})
	if err != nil || input.BearerToken != "secret-value" {
		t.Fatalf("input=%#v err=%v", input, err)
	}
	ref := plugindomain.ManagedCredentialReference("github")
	if !pluginCredentialRefRE.MatchString(ref) || !plugindomain.IsManagedCredentialReference("github", ref) {
		t.Fatalf("invalid managed credential reference %q", ref)
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

func TestPluginConnectionDTOUsesExecutionRouter(t *testing.T) {
	dto, err := pluginConnectionDTOFrom(cloud.PluginConnection{DeviceID: "mac-1", WorkspaceKey: "workspace", ServerName: "plugin-github"})
	if err != nil {
		t.Fatal(err)
	}
	if dto.Connection != "local:mac-1" || dto.ExecutionTarget != "local" {
		t.Fatalf("unexpected routed connection DTO: %#v", dto)
	}
}
