// Package cloudmcp provides bounded, request-scoped MCP transport. Durable
// configuration, credentials and user approval belong to the Cloud store.
package cloudmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxTools = 500
const maxSchemaBytes = 128 << 10
const maxArgumentsBytes = 64 << 10

type ToolSummary struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	ReadOnly     bool            `json:"readOnlyHint"`
}

type Config struct{ Endpoint, Bearer string }

type Manager struct {
	mu     sync.Mutex
	active map[string]int
	total  int
	client func(string, string) (*http.Client, func())
}

func NewManager() *Manager { return &Manager{active: map[string]int{}, client: newHTTPClient} }

func (m *Manager) acquire(user string) (func(), error) {
	if strings.TrimSpace(user) == "" {
		return nil, errors.New("MCP account is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total >= 64 || m.active[user] >= 4 {
		return nil, errors.New("MCP concurrent operation limit reached")
	}
	m.active[user]++
	m.total++
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.active[user]--
		m.total--
		if m.active[user] == 0 {
			delete(m.active, user)
		}
	}, nil
}

func (m *Manager) open(ctx context.Context, user string, cfg Config) (*mcp.ClientSession, func(), error) {
	endpoint, err := ValidateEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, nil, err
	}
	if len(cfg.Bearer) > 16384 || strings.ContainsAny(cfg.Bearer, "\r\n\x00") {
		return nil, nil, errors.New("invalid MCP credential")
	}
	release, err := m.acquire(user)
	if err != nil {
		return nil, nil, err
	}
	httpClient, closeHTTP := m.client(endpoint, cfg.Bearer)
	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-cloud", Version: "2.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		closeHTTP()
		release()
		return nil, nil, errors.New("MCP connection failed; verify endpoint and authorization")
	}
	return session, func() { _ = session.Close(); closeHTTP(); release() }, nil
}

func schemaBytes(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxSchemaBytes || len(data) == 0 || data[0] != '{' {
		return nil, errors.New("MCP tool has an invalid or oversized schema")
	}
	return data, nil
}

func discover(ctx context.Context, session *mcp.ClientSession) ([]ToolSummary, error) {
	tools := []ToolSummary{}
	seen := map[string]bool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, errors.New("MCP tool discovery failed")
		}
		if len(tools) >= maxTools {
			return nil, errors.New("MCP tool catalog exceeds limit")
		}
		if tool.Name == "" || len(tool.Name) > 128 || seen[tool.Name] {
			return nil, errors.New("MCP tool name is invalid or duplicated")
		}
		seen[tool.Name] = true
		input, err := schemaBytes(tool.InputSchema)
		if err != nil {
			return nil, err
		}
		var output json.RawMessage
		if tool.OutputSchema != nil {
			output, err = schemaBytes(tool.OutputSchema)
			if err != nil {
				return nil, err
			}
		}
		description := tool.Description
		if len(description) > 4096 {
			description = description[:4096]
		}
		item := ToolSummary{Name: tool.Name, Description: description, InputSchema: input, OutputSchema: output}
		if tool.Annotations != nil {
			item.ReadOnly = tool.Annotations.ReadOnlyHint
		}
		tools = append(tools, item)
	}
	return tools, nil
}

func (m *Manager) Discover(ctx context.Context, user string, cfg Config) ([]ToolSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	session, closeSession, err := m.open(ctx, user, cfg)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	return discover(ctx, session)
}

func validateArguments(schema json.RawMessage, arguments map[string]any) error {
	raw, err := json.Marshal(arguments)
	if err != nil || len(raw) > maxArgumentsBytes {
		return errors.New("MCP arguments exceed limit")
	}
	var definition jsonschema.Schema
	if json.Unmarshal(schema, &definition) != nil {
		return errors.New("invalid MCP input schema")
	}
	resolved, err := definition.Resolve(nil)
	if err != nil {
		return errors.New("MCP input schema cannot be resolved locally")
	}
	var instance any
	if json.Unmarshal(raw, &instance) != nil || resolved.Validate(instance) != nil {
		return errors.New("MCP arguments do not match input schema")
	}
	return nil
}

// CallApproved is invoked only after consuming a backend-owned one-shot
// approval. MCP annotations never authorize execution, including readOnlyHint.
func (m *Manager) CallApproved(ctx context.Context, user string, cfg Config, name string, arguments map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	session, closeSession, err := m.open(ctx, user, cfg)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	tools, err := discover(ctx, session)
	if err != nil {
		return nil, err
	}
	var selected *ToolSummary
	for i := range tools {
		if tools[i].Name == name {
			selected = &tools[i]
			break
		}
	}
	if selected == nil {
		return nil, errors.New("MCP tool is no longer available")
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	if err := validateArguments(selected.InputSchema, arguments); err != nil {
		return nil, err
	}
	// A failure after dispatch has an unknown outcome. Never replay this call.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, errors.New("MCP call interrupted; outcome unknown, do not retry automatically")
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > maxResponseBytes {
		return nil, errors.New("MCP result exceeds limit")
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil, errors.New("MCP result is invalid")
	}
	return out, nil
}
