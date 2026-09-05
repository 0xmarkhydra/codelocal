package localclient

import "testing"

func TestPluginMCPConfigUsesGlobalHTTPAndCredentialReference(t *testing.T) {
	config, err := pluginMCPConfig(map[string]any{
		"pluginId":  "github",
		"endpoint":  "https://mcp.example.com/mcp",
		"bearerEnv": "GITHUB_TOKEN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "plugin-github" || config.Scope != "global" || config.Transport != "http" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.URL != "https://mcp.example.com/mcp" {
		t.Fatalf("endpoint=%q", config.URL)
	}
	auth := config.Headers["Authorization"]
	if auth.Source != "GITHUB_TOKEN" || auth.Prefix != "Bearer " {
		t.Fatalf("authorization ref=%#v", auth)
	}
}

func TestPluginMCPConfigRejectsSecretsInURLAndInvalidEnv(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"url credentials": {"pluginId": "github", "endpoint": "https://token@example.com/mcp"},
		"public http":     {"pluginId": "github", "endpoint": "http://example.com/mcp"},
		"invalid env":     {"pluginId": "github", "endpoint": "https://example.com/mcp", "bearerEnv": "TOKEN=value"},
		"bad plugin id":   {"pluginId": "GitHub Plugin", "endpoint": "https://example.com/mcp"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pluginMCPConfig(args); err == nil {
				t.Fatalf("expected rejection for %#v", args)
			}
		})
	}
}

func TestPluginMCPConfigAllowsLoopbackHTTP(t *testing.T) {
	config, err := pluginMCPConfig(map[string]any{"pluginId": "dev-mcp", "endpoint": "http://127.0.0.1:3001/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if config.URL != "http://127.0.0.1:3001/mcp" {
		t.Fatalf("endpoint=%q", config.URL)
	}
}

func TestPluginMCPServerNameIsStableAndBounded(t *testing.T) {
	longID := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghij"
	first, err := pluginMCPServerName(longID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pluginMCPServerName(longID)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) > 64 {
		t.Fatalf("server name=%q second=%q", first, second)
	}
}
