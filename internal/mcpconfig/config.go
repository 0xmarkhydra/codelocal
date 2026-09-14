package mcpconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type Mode string

const (
	ModeLocal  Mode = "local"
	ModeOnline Mode = "online"
)

type Reference struct {
	Source string `json:"source"`
	Prefix string `json:"prefix,omitempty"`
}

type Server struct {
	Name      string               `json:"name"`
	Enabled   bool                 `json:"enabled"`
	Transport string               `json:"transport"`
	Command   string               `json:"command,omitempty"`
	Args      []string             `json:"args,omitempty"`
	CWD       string               `json:"cwd,omitempty"`
	Env       map[string]Reference `json:"env,omitempty"`
	URL       string               `json:"url,omitempty"`
	Headers   map[string]Reference `json:"headers,omitempty"`
}

type MaterializedServer struct {
	Server  Server            `json:"server"`
	Secrets map[string]string `json:"secrets,omitempty"`
}

type Actual struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	ToolCount int    `json:"toolCount"`
	LastError string `json:"lastError,omitempty"`
}

type rawServer struct {
	Type      string            `json:"type"`
	Transport string            `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	CWD       string            `json:"cwd"`
	URL       string            `json:"url"`
	Enabled   *bool             `json:"enabled"`
	Env       map[string]string `json:"env"`
	Headers   map[string]string `json:"headers"`
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var refRE = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
var prefixedRefRE = regexp.MustCompile(`^(.*)\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

func Parse(raw string, mode Mode) ([]Server, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("invalid MCP JSON: %w", err)
	}
	if len(root) == 0 {
		return nil, errors.New("mcp JSON is empty")
	}
	serversRaw := root
	if wrapped, ok := root["mcpServers"]; ok {
		serversRaw = map[string]json.RawMessage{}
		if err := json.Unmarshal(wrapped, &serversRaw); err != nil {
			return nil, errors.New("mcpServers must be an object")
		}
	} else if wrapped, ok := root["servers"]; ok {
		serversRaw = map[string]json.RawMessage{}
		if err := json.Unmarshal(wrapped, &serversRaw); err != nil {
			return nil, errors.New("servers must be an object")
		}
	}
	if len(serversRaw) == 0 {
		return nil, errors.New("no MCP servers found")
	}
	names := make([]string, 0, len(serversRaw))
	for name := range serversRaw {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Server, 0, len(names))
	for _, name := range names {
		if !nameRE.MatchString(name) {
			return nil, fmt.Errorf("invalid MCP server name %q", name)
		}
		var input rawServer
		if err := json.Unmarshal(serversRaw[name], &input); err != nil {
			return nil, fmt.Errorf("MCP server %q must be an object", name)
		}
		server, err := normalizeServer(name, input, mode)
		if err != nil {
			return nil, fmt.Errorf("MCP server %q: %w", name, err)
		}
		out = append(out, server)
	}
	return out, nil
}

func normalizeServer(name string, input rawServer, mode Mode) (Server, error) {
	server := Server{Name: name, Enabled: true, Command: strings.TrimSpace(input.Command), Args: append([]string(nil), input.Args...), CWD: strings.TrimSpace(input.CWD), URL: strings.TrimSpace(input.URL)}
	if input.Enabled != nil {
		server.Enabled = *input.Enabled
	}
	transport := strings.ToLower(strings.TrimSpace(input.Transport))
	if transport == "" {
		transport = strings.ToLower(strings.TrimSpace(input.Type))
	}
	if server.Command != "" {
		if mode == ModeOnline {
			return Server{}, errors.New("online MCP servers must use a URL, not a command")
		}
		if transport != "" && transport != "stdio" {
			return Server{}, errors.New("command MCP servers must use stdio transport")
		}
		server.Transport = "stdio"
		if server.URL != "" {
			return Server{}, errors.New("provide command or url, not both")
		}
	} else if server.URL != "" {
		switch transport {
		case "", "http", "streamable-http", "sse":
			server.Transport = "http"
		default:
			return Server{}, fmt.Errorf("unsupported remote transport %q", transport)
		}
		parsed, err := url.Parse(server.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return Server{}, errors.New("url must be an absolute HTTP(S) URL")
		}
		if parsed.User != nil || parsed.Fragment != "" {
			return Server{}, errors.New("url cannot contain credentials or a fragment")
		}
	} else {
		return Server{}, errors.New("server requires command or url")
	}
	server.Env = map[string]Reference{}
	for key, value := range input.Env {
		key = strings.TrimSpace(key)
		if !nameRE.MatchString(key) {
			return Server{}, fmt.Errorf("invalid env name %q", key)
		}
		ref, err := strictReference(value)
		if err != nil {
			return Server{}, fmt.Errorf("env %s: %w", key, err)
		}
		server.Env[key] = ref
	}
	server.Headers = map[string]Reference{}
	for header, value := range input.Headers {
		header = strings.TrimSpace(header)
		if header == "" || strings.ContainsAny(header, "\r\n") {
			return Server{}, errors.New("invalid header name")
		}
		ref, err := prefixedReference(value)
		if err != nil {
			return Server{}, fmt.Errorf("header %s: %w", header, err)
		}
		server.Headers[header] = ref
	}
	if mode == ModeOnline && server.Transport != "http" {
		return Server{}, errors.New("online MCP supports remote HTTP servers only")
	}
	if mode == ModeOnline && len(server.Env) > 0 {
		return Server{}, errors.New("online MCP cannot use local environment variables")
	}
	return server, nil
}

func strictReference(value string) (Reference, error) {
	value = strings.TrimSpace(value)
	match := refRE.FindStringSubmatch(value)
	if len(match) != 2 {
		return Reference{}, errors.New("secret values must use an environment reference")
	}
	return Reference{Source: match[1]}, nil
}

func prefixedReference(value string) (Reference, error) {
	value = strings.TrimSpace(value)
	match := prefixedRefRE.FindStringSubmatch(value)
	if len(match) != 3 {
		return Reference{}, errors.New("secret header values must contain exactly one environment reference")
	}
	if strings.Contains(match[1], "${") {
		return Reference{}, errors.New("header may contain only one environment reference")
	}
	return Reference{Source: match[2], Prefix: match[1]}, nil
}
