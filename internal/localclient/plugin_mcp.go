package localclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/mcphub"
)

var pluginEnvNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func pluginMCPServerName(pluginID string) (string, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return "", errors.New("plugin id is required")
	}
	for _, r := range pluginID {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return "", errors.New("plugin id is not canonical")
		}
	}
	name := "plugin-" + pluginID
	if len(name) <= 64 {
		return name, nil
	}
	digest := sha256.Sum256([]byte(pluginID))
	suffix := hex.EncodeToString(digest[:])[:10]
	prefix := pluginID
	maxPrefix := 64 - len("plugin--") - len(suffix)
	if len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}
	return "plugin-" + prefix + "-" + suffix, nil
}

func validatePluginEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("plugin MCP endpoint must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("plugin MCP endpoint cannot include credentials or a fragment")
	}
	host := strings.Trim(strings.ToLower(parsed.Hostname()), "[]")
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", errors.New("plugin MCP endpoint must use HTTPS unless it is localhost")
	}
	return parsed.String(), nil
}

func pluginMCPConfig(args map[string]any) (mcphub.ServerConfig, error) {
	name, err := pluginMCPServerName(asString(args["pluginId"]))
	if err != nil {
		return mcphub.ServerConfig{}, err
	}
	endpoint, err := validatePluginEndpoint(asString(args["endpoint"]))
	if err != nil {
		return mcphub.ServerConfig{}, err
	}
	config := mcphub.ServerConfig{
		Name:      name,
		Enabled:   true,
		Scope:     "global",
		Transport: "http",
		URL:       endpoint,
	}
	bearerEnv := strings.TrimSpace(asString(args["bearerEnv"]))
	if bearerEnv != "" {
		if !pluginEnvNameRE.MatchString(bearerEnv) {
			return mcphub.ServerConfig{}, errors.New("bearer environment variable name is invalid")
		}
		config.Headers = map[string]mcphub.HeaderReference{
			"Authorization": {Source: bearerEnv, Prefix: "Bearer "},
		}
	}
	return config, nil
}

func (e *Engine) configurePluginMCP(ctx context.Context, args map[string]any) (any, error) {
	config, err := pluginMCPConfig(args)
	if err != nil {
		return nil, err
	}
	server, err := e.MCP.Add(config)
	if err != nil {
		return nil, err
	}
	probe, probeErr := e.MCP.Probe(ctx, config.Name, true)
	if probeErr != nil {
		return map[string]any{
			"configured": true,
			"connected":  false,
			"serverName": config.Name,
			"server":     server,
			"error":      probeErr.Error(),
		}, nil
	}
	toolCount := 0
	if value, ok := probe["toolCount"].(int); ok {
		toolCount = value
	}
	return map[string]any{
		"configured": true,
		"connected":  true,
		"serverName": config.Name,
		"server":     server,
		"toolCount":  toolCount,
	}, nil
}

func (e *Engine) removePluginMCP(args map[string]any) (any, error) {
	name, err := pluginMCPServerName(asString(args["pluginId"]))
	if err != nil {
		return nil, err
	}
	result, err := e.MCP.Remove(name, "global")
	if err != nil {
		return nil, err
	}
	return map[string]any{"removed": result["removed"], "serverName": name}, nil
}
