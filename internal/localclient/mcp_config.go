package localclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/mcpconfig"
	"github.com/0xmarkhydra/codelocal/internal/mcphub"
)

func decodeMCPServer(value any) (mcpconfig.Server, error) {
	var server mcpconfig.Server
	raw, err := json.Marshal(value)
	if err != nil {
		return server, err
	}
	if err := json.Unmarshal(raw, &server); err != nil {
		return server, err
	}
	if strings.TrimSpace(server.Name) == "" {
		return server, errors.New("MCP server name is required")
	}
	return server, nil
}

func decodeMCPSecrets(value any) map[string]string {
	out := map[string]string{}
	root, _ := value.(map[string]any)
	for key, raw := range root {
		if text, ok := raw.(string); ok && strings.TrimSpace(key) != "" && text != "" {
			out[key] = text
		}
	}
	return out
}

func mcpManagedSecretRef(server, source string) string {
	sum := sha256.Sum256([]byte(server + "|" + source))
	return "CODELOCAL_MCP_" + strings.ToUpper(hex.EncodeToString(sum[:8]))
}

func (e *Engine) mcpServerConfig(server mcpconfig.Server, secrets map[string]string) mcphub.ServerConfig {
	config := mcphub.ServerConfig{
		Name:      server.Name,
		Enabled:   server.Enabled,
		Managed:   true,
		Scope:     "global",
		Transport: server.Transport,
		Command:   server.Command,
		Args:      append([]string(nil), server.Args...),
		CWD:       server.CWD,
		URL:       server.URL,
	}
	if len(server.Env) > 0 {
		config.Env = map[string]mcphub.EnvReference{}
		for target, ref := range server.Env {
			source := ref.Source
			if secret, ok := secrets[ref.Source]; ok {
				source = mcpManagedSecretRef(server.Name, ref.Source)
				e.setRuntimeSecret(source, secret)
			}
			config.Env[target] = mcphub.EnvReference{Source: source}
		}
	}
	if len(server.Headers) > 0 {
		config.Headers = map[string]mcphub.HeaderReference{}
		for header, ref := range server.Headers {
			source := ref.Source
			if secret, ok := secrets[ref.Source]; ok {
				source = mcpManagedSecretRef(server.Name, ref.Source)
				e.setRuntimeSecret(source, secret)
			}
			config.Headers[header] = mcphub.HeaderReference{Source: source, Prefix: ref.Prefix}
		}
	}
	return config
}

func (e *Engine) applyMCPServer(ctx context.Context, server mcpconfig.Server, secrets map[string]string) mcpconfig.Actual {
	config := e.mcpServerConfig(server, secrets)
	actual := mcpconfig.Actual{Name: server.Name, State: "configured"}
	if _, err := e.MCP.AddManaged(config); err != nil {
		actual.State, actual.LastError = "error", err.Error()
		return actual
	}
	if !server.Enabled {
		return actual
	}
	probe, err := e.MCP.Probe(ctx, server.Name, true)
	if err != nil {
		actual.State, actual.LastError = "error", err.Error()
		return actual
	}
	actual.State = "ready"
	actual.ToolCount = asInt(probe["toolCount"], 0)
	return actual
}

func (e *Engine) configureMCP(ctx context.Context, args map[string]any) (any, error) {
	server, err := decodeMCPServer(args["server"])
	if err != nil {
		return nil, err
	}
	actual := e.applyMCPServer(ctx, server, decodeMCPSecrets(args["secrets"]))
	info, infoErr := e.MCP.Info(server.Name)
	if infoErr != nil {
		info = map[string]any{"name": server.Name}
	}
	return map[string]any{
		"configured": true,
		"connected":  actual.State == "ready",
		"serverName": server.Name,
		"toolCount":  actual.ToolCount,
		"error":      actual.LastError,
		"server":     info,
	}, nil
}

func (e *Engine) removeMCP(args map[string]any) (any, error) {
	name := strings.TrimSpace(asString(args["name"]))
	if name == "" {
		return nil, errors.New("MCP server name is required")
	}
	info, err := e.MCP.Info(name)
	if err != nil {
		return e.MCP.Remove(name, "global")
	}
	managed, _ := info["managed"].(bool)
	if !managed {
		return map[string]any{"removed": 0, "serverName": name, "skipped": "user_managed"}, nil
	}
	return e.MCP.Remove(name, "global")
}

func (e *Engine) ReconcileMCPServers(ctx context.Context, desired []mcpconfig.MaterializedServer) []mcpconfig.Actual {
	wanted := make(map[string]struct{}, len(desired))
	actual := make([]mcpconfig.Actual, 0, len(desired))
	for _, item := range desired {
		wanted[item.Server.Name] = struct{}{}
		actual = append(actual, e.applyMCPServer(ctx, item.Server, item.Secrets))
	}
	listed, err := e.MCP.List()
	if err != nil {
		return actual
	}
	items, _ := listed.([]map[string]any)
	for _, item := range items {
		name, _ := item["name"].(string)
		scope, _ := item["scope"].(string)
		managed, _ := item["managed"].(bool)
		if !managed || scope != "global" {
			continue
		}
		if _, keep := wanted[name]; !keep {
			_, _ = e.MCP.Remove(name, "global")
		}
	}
	return actual
}
