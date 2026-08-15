package mcphub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type EnvReference struct {
	Source string `json:"source"`
}
type HeaderReference struct {
	Source string `json:"source"`
	Prefix string `json:"prefix,omitempty"`
}
type ServerConfig struct {
	Name          string                     `json:"name"`
	Enabled       bool                       `json:"enabled"`
	Scope         string                     `json:"scope"`
	WorkspaceRoot string                     `json:"workspaceRoot,omitempty"`
	Transport     string                     `json:"transport"`
	Command       string                     `json:"command,omitempty"`
	Args          []string                   `json:"args,omitempty"`
	CWD           string                     `json:"cwd,omitempty"`
	Env           map[string]EnvReference    `json:"env,omitempty"`
	URL           string                     `json:"url,omitempty"`
	Headers       map[string]HeaderReference `json:"headers,omitempty"`
	AddedAt       int64                      `json:"addedAt"`
	UpdatedAt     int64                      `json:"updatedAt"`
}
type CatalogTool struct {
	ServerKey    string `json:"serverKey"`
	Server       string `json:"server"`
	Name         string `json:"name"`
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	InputSchema  any    `json:"inputSchema,omitempty"`
	OutputSchema any    `json:"outputSchema,omitempty"`
	Annotations  any    `json:"annotations,omitempty"`
	DiscoveredAt int64  `json:"discoveredAt"`
}
type registryFile struct {
	Version int            `json:"version"`
	Servers []ServerConfig `json:"servers"`
}
type catalogFile struct {
	Version int           `json:"version"`
	Tools   []CatalogTool `json:"tools"`
}
type connected struct {
	session                 *mcp.ClientSession
	connectedAt, lastUsedAt int64
}
type ConnectGuard func(ServerConfig) error

type Hub struct {
	Root       string
	guard      ConnectGuard
	mu         sync.Mutex
	sessions   map[string]*connected
	connecting map[string]chan struct{}
	close      chan struct{}
	once       sync.Once
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var envRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func statePaths() (string, string, string) {
	dir := filepath.Join(state.Dir(), "mcp")
	return dir, filepath.Join(dir, "registry.json"), filepath.Join(dir, "catalog.json")
}
func New(root string, guard ConnectGuard) (*Hub, error) {
	real, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	h := &Hub{Root: real, guard: guard, sessions: map[string]*connected{}, connecting: map[string]chan struct{}{}, close: make(chan struct{})}
	go h.sweep()
	return h, nil
}
func (h *Hub) sweep() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	idle := 10 * time.Minute
	for {
		select {
		case <-h.close:
			return
		case <-ticker.C:
			h.mu.Lock()
			names := []string{}
			cut := time.Now().Add(-idle).UnixMilli()
			for name, s := range h.sessions {
				if s.lastUsedAt < cut {
					names = append(names, name)
				}
			}
			h.mu.Unlock()
			for _, name := range names {
				_ = h.disconnect(name)
			}
		}
	}
}
func readRegistry() (registryFile, error) {
	_, file, _ := statePaths()
	var value registryFile
	if err := state.ReadJSON(file, &value); err != nil {
		if os.IsNotExist(err) {
			return registryFile{Version: 1}, nil
		}
		return value, err
	}
	if value.Version != 1 {
		return value, errors.New("unsupported MCP registry format")
	}
	return value, nil
}
func readCatalog() (catalogFile, error) {
	_, _, file := statePaths()
	var value catalogFile
	if err := state.ReadJSON(file, &value); err != nil {
		if os.IsNotExist(err) {
			return catalogFile{Version: 1}, nil
		}
		return value, err
	}
	if value.Version != 1 {
		return value, errors.New("unsupported MCP catalog format")
	}
	return value, nil
}
func configKey(s ServerConfig) string {
	if s.Scope == "global" {
		return "global:" + s.Name
	}
	sum := sha256.Sum256([]byte(s.WorkspaceRoot))
	return "workspace:" + s.Name + ":" + hex.EncodeToString(sum[:])[:20]
}
func validName(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !nameRE.MatchString(v) {
		return "", errors.New("MCP name must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}")
	}
	return v, nil
}
func validURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("remote MCP URL must use http or https")
	}
	host := strings.Trim(strings.ToLower(u.Hostname()), "[]")
	loop := host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0"
	if u.Scheme == "http" && !loop && os.Getenv("CODELOCAL_MCP_ALLOW_INSECURE_HTTP") != "1" {
		return "", errors.New("remote MCP must use HTTPS unless localhost")
	}
	return u.String(), nil
}
func normalize(root string, in ServerConfig, old *ServerConfig) (ServerConfig, error) {
	name, err := validName(in.Name)
	if err != nil {
		return in, err
	}
	in.Name = name
	if in.Scope != "global" && in.Scope != "workspace" {
		return in, errors.New("MCP scope must be global or workspace")
	}
	if in.Transport != "stdio" && in.Transport != "http" {
		return in, errors.New("MCP transport must be stdio or http")
	}
	if in.Transport == "stdio" {
		if strings.TrimSpace(in.Command) == "" {
			return in, errors.New("stdio MCP requires command")
		}
		in.URL = ""
	} else {
		in.URL, err = validURL(in.URL)
		if err != nil {
			return in, err
		}
		in.Command = ""
		in.Args = nil
	}
	if in.Scope == "workspace" {
		if in.WorkspaceRoot == "" {
			in.WorkspaceRoot = root
		}
		in.WorkspaceRoot, err = filepath.Abs(in.WorkspaceRoot)
		if err != nil {
			return in, err
		}
	} else {
		in.WorkspaceRoot = ""
	}
	now := time.Now().UnixMilli()
	in.AddedAt = now
	if old != nil {
		in.AddedAt = old.AddedAt
	}
	in.UpdatedAt = now
	if !in.Enabled && old == nil {
		in.Enabled = true
	}
	return in, nil
}
func effective(reg registryFile, root string) []ServerConfig {
	by := map[string]ServerConfig{}
	for _, s := range reg.Servers {
		if s.Scope == "global" {
			by[s.Name] = s
		}
	}
	for _, s := range reg.Servers {
		if s.Scope == "workspace" && s.WorkspaceRoot == root {
			by[s.Name] = s
		}
	}
	out := []ServerConfig{}
	for _, s := range by {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (h *Hub) resolve(name string) (ServerConfig, error) {
	reg, err := readRegistry()
	if err != nil {
		return ServerConfig{}, err
	}
	for i := len(reg.Servers) - 1; i >= 0; i-- {
		s := reg.Servers[i]
		if s.Name == name && s.Scope == "workspace" && s.WorkspaceRoot == h.Root {
			if !s.Enabled {
				return s, errors.New("MCP server disabled")
			}
			return s, nil
		}
	}
	for _, s := range reg.Servers {
		if s.Name == name && s.Scope == "global" {
			if !s.Enabled {
				return s, errors.New("MCP server disabled")
			}
			return s, nil
		}
	}
	return ServerConfig{}, fmt.Errorf("MCP server not installed: %s", name)
}
func public(s ServerConfig) map[string]any {
	env := map[string]string{}
	for k, v := range s.Env {
		env[k] = v.Source
	}
	headers := map[string]any{}
	for k, v := range s.Headers {
		headers[k] = map[string]any{"source": v.Source, "prefix": func() string {
			if v.Prefix != "" {
				return "[configured]"
			}
			return ""
		}()}
	}
	return map[string]any{"name": s.Name, "enabled": s.Enabled, "scope": s.Scope, "workspaceRoot": s.WorkspaceRoot, "transport": s.Transport, "command": s.Command, "args": s.Args, "cwd": s.CWD, "env": env, "url": s.URL, "headers": headers, "addedAt": s.AddedAt, "updatedAt": s.UpdatedAt}
}
func (h *Hub) Add(config ServerConfig) (map[string]any, error) {
	reg, err := readRegistry()
	if err != nil {
		return nil, err
	}
	var old *ServerConfig
	for i := range reg.Servers {
		s := &reg.Servers[i]
		if s.Name == config.Name && s.Scope == config.Scope && (s.Scope == "global" || s.WorkspaceRoot == config.WorkspaceRoot) {
			old = s
			break
		}
	}
	normalized, err := normalize(h.Root, config, old)
	if err != nil {
		return nil, err
	}
	next := []ServerConfig{}
	for _, s := range reg.Servers {
		if s.Name == normalized.Name && s.Scope == normalized.Scope && (s.Scope == "global" || s.WorkspaceRoot == normalized.WorkspaceRoot) {
			continue
		}
		next = append(next, s)
	}
	next = append(next, normalized)
	sort.Slice(next, func(i, j int) bool { return next[i].Scope+next[i].Name < next[j].Scope+next[j].Name })
	reg.Servers = next
	_, file, _ := statePaths()
	if err := state.WriteJSONAtomic(file, reg); err != nil {
		return nil, err
	}
	_ = h.disconnect(normalized.Name)
	return public(normalized), nil
}
func (h *Hub) Remove(name, scope string) (map[string]any, error) {
	reg, err := readRegistry()
	if err != nil {
		return nil, err
	}
	removed := []ServerConfig{}
	next := []ServerConfig{}
	for _, s := range reg.Servers {
		match := s.Name == name && (scope == "" || s.Scope == scope) && (s.Scope != "workspace" || s.WorkspaceRoot == h.Root)
		if match {
			removed = append(removed, s)
		} else {
			next = append(next, s)
		}
	}
	reg.Servers = next
	if len(removed) > 0 {
		_, file, _ := statePaths()
		if err := state.WriteJSONAtomic(file, reg); err != nil {
			return nil, err
		}
		cat, _ := readCatalog()
		keys := map[string]struct{}{}
		for _, s := range removed {
			keys[configKey(s)] = struct{}{}
		}
		tools := cat.Tools[:0]
		for _, t := range cat.Tools {
			if _, ok := keys[t.ServerKey]; !ok {
				tools = append(tools, t)
			}
		}
		cat.Tools = tools
		_, _, catalogPath := statePaths()
		_ = state.WriteJSONAtomic(catalogPath, cat)
	}
	_ = h.disconnect(name)
	return map[string]any{"removed": len(removed)}, nil
}
func (h *Hub) List() (any, error) {
	reg, err := readRegistry()
	if err != nil {
		return nil, err
	}
	cat, _ := readCatalog()
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []map[string]any{}
	for _, s := range effective(reg, h.Root) {
		p := public(s)
		count := 0
		for _, t := range cat.Tools {
			if t.ServerKey == configKey(s) {
				count++
			}
		}
		p["toolsCached"] = count
		p["connected"] = h.sessions[s.Name] != nil
		out = append(out, p)
	}
	return out, nil
}

func (h *Hub) Info(name string) (map[string]any, error) {
	config, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	cat, _ := readCatalog()
	key := configKey(config)
	tools := []CatalogTool{}
	for _, tool := range cat.Tools {
		if tool.ServerKey == key {
			tools = append(tools, tool)
		}
	}
	h.mu.Lock()
	connected := h.sessions[name] != nil
	h.mu.Unlock()
	result := public(config)
	result["connected"] = connected
	result["tools"] = tools
	result["toolsCached"] = len(tools)
	return result, nil
}
func materializeEnv(refs map[string]EnvReference) ([]string, error) {
	env := os.Environ()
	for target, ref := range refs {
		if !envRE.MatchString(target) || !envRE.MatchString(ref.Source) {
			return nil, errors.New("invalid MCP environment variable name")
		}
		value, ok := os.LookupEnv(ref.Source)
		if !ok {
			return nil, fmt.Errorf("MCP requires environment variable %s", ref.Source)
		}
		env = append(env, target+"="+value)
	}
	return env, nil
}
func materializeHeaders(refs map[string]HeaderReference) (http.Header, error) {
	headers := http.Header{}
	for key, ref := range refs {
		if !envRE.MatchString(ref.Source) {
			return nil, errors.New("invalid MCP header environment reference")
		}
		value, ok := os.LookupEnv(ref.Source)
		if !ok {
			return nil, fmt.Errorf("MCP requires environment variable %s", ref.Source)
		}
		headers.Set(key, ref.Prefix+value)
	}
	return headers, nil
}
func (h *Hub) connect(ctx context.Context, config ServerConfig, authorize bool) (*mcp.ClientSession, error) {
	h.mu.Lock()
	if s := h.sessions[config.Name]; s != nil {
		s.lastUsedAt = time.Now().UnixMilli()
		h.mu.Unlock()
		return s.session, nil
	}
	if ch := h.connecting[config.Name]; ch != nil {
		h.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
			return h.connect(ctx, config, authorize)
		}
	}
	ch := make(chan struct{})
	h.connecting[config.Name] = ch
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.connecting, config.Name); close(ch); h.mu.Unlock() }()
	if !authorize && h.guard != nil {
		if err := h.guard(config); err != nil {
			return nil, err
		}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-mcp-hub", Version: "2.0.0"}, nil)
	var transport mcp.Transport
	if config.Transport == "stdio" {
		cwd := config.CWD
		if cwd == "" && config.Scope == "workspace" {
			cwd = h.Root
		} else if cwd != "" && !filepath.IsAbs(cwd) {
			cwd = filepath.Join(h.Root, cwd)
		}
		cmd := exec.Command(config.Command, config.Args...)
		cmd.Dir = cwd
		env, err := materializeEnv(config.Env)
		if err != nil {
			return nil, err
		}
		cmd.Env = env
		transport = &mcp.CommandTransport{Command: cmd}
	} else {
		headers, err := materializeHeaders(config.Headers)
		if err != nil {
			return nil, err
		}
		httpClient := &http.Client{Timeout: 0, Transport: &headerTransport{base: http.DefaultTransport, headers: headers}}
		transport = &mcp.StreamableClientTransport{Endpoint: config.URL, HTTPClient: httpClient}
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect MCP %s: %w", config.Name, err)
	}
	h.mu.Lock()
	h.sessions[config.Name] = &connected{session: session, connectedAt: time.Now().UnixMilli(), lastUsedAt: time.Now().UnixMilli()}
	h.mu.Unlock()
	return session, nil
}

type headerTransport struct {
	base    http.RoundTripper
	headers http.Header
}

func (t *headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	for k, v := range t.headers {
		clone.Header[k] = append([]string(nil), v...)
	}
	return t.base.RoundTrip(clone)
}
func (h *Hub) disconnect(name string) error {
	h.mu.Lock()
	s := h.sessions[name]
	delete(h.sessions, name)
	h.mu.Unlock()
	if s != nil {
		return s.session.Close()
	}
	return nil
}
func (h *Hub) Probe(ctx context.Context, name string, authorize bool) (map[string]any, error) {
	config, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	session, err := h.connect(ctx, config, authorize)
	if err != nil {
		return nil, err
	}
	tools := []CatalogTool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		tools = append(tools, CatalogTool{ServerKey: configKey(config), Server: config.Name, Name: tool.Name, Title: tool.Title, Description: tool.Description, InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema, Annotations: tool.Annotations, DiscoveredAt: time.Now().UnixMilli()})
		if len(tools) >= 5000 {
			break
		}
	}
	cat, _ := readCatalog()
	next := cat.Tools[:0]
	for _, t := range cat.Tools {
		if t.ServerKey != configKey(config) {
			next = append(next, t)
		}
	}
	cat.Tools = append(next, tools...)
	_, _, path := statePaths()
	if err := state.WriteJSONAtomic(path, cat); err != nil {
		return nil, err
	}
	preview := []map[string]any{}
	for i, t := range tools {
		if i >= 100 {
			break
		}
		preview = append(preview, map[string]any{"name": t.Name, "title": t.Title, "description": t.Description})
	}
	return map[string]any{"server": public(config), "connected": true, "toolCount": len(tools), "tools": preview, "truncated": len(tools) > 100}, nil
}
func score(t CatalogTool, q string) int {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return 1
	}
	name := strings.ToLower(t.Server + "." + t.Name)
	title := strings.ToLower(t.Title)
	desc := strings.ToLower(t.Description)
	s := 0
	if name == q || strings.ToLower(t.Name) == q {
		s += 100
	}
	if strings.Contains(name, q) {
		s += 40
	}
	if strings.Contains(title, q) {
		s += 24
	}
	if strings.Contains(desc, q) {
		s += 12
	}
	for _, term := range strings.FieldsFunc(q, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("_./:-", r))
	}) {
		if strings.Contains(strings.ToLower(t.Name), term) {
			s += 14
		}
		if strings.Contains(desc, term) {
			s += 4
		}
	}
	return s
}
func (h *Hub) Search(ctx context.Context, query, server string, limit int, refresh bool) (map[string]any, error) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}
	if refresh && server != "" {
		if _, err := h.Probe(ctx, server, false); err != nil {
			return nil, err
		}
	}
	reg, err := readRegistry()
	if err != nil {
		return nil, err
	}
	visible := effective(reg, h.Root)
	allowed := map[string]struct{}{}
	for _, s := range visible {
		if s.Enabled {
			allowed[configKey(s)] = struct{}{}
		}
	}
	cat, _ := readCatalog()
	type ranked struct {
		CatalogTool
		Score int
	}
	items := []ranked{}
	for _, t := range cat.Tools {
		if _, ok := allowed[t.ServerKey]; !ok || server != "" && t.Server != server {
			continue
		}
		n := score(t, query)
		if strings.TrimSpace(query) != "" && n == 0 {
			continue
		}
		items = append(items, ranked{t, n})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return items[i].Server+items[i].Name < items[j].Server+items[j].Name
	})
	if len(items) > limit {
		items = items[:limit]
	}
	results := []map[string]any{}
	for _, t := range items {
		results = append(results, map[string]any{"server": t.Server, "tool": t.Name, "title": t.Title, "description": t.Description, "score": t.Score})
	}
	return map[string]any{"query": query, "results": results, "catalogToolCount": len(cat.Tools), "installedServerCount": len(visible)}, nil
}
func (h *Hub) ToolInfo(ctx context.Context, server, tool string, authorize bool) (*CatalogTool, error) {
	config, err := h.resolve(server)
	if err != nil {
		return nil, err
	}
	key := configKey(config)
	cat, _ := readCatalog()
	for i := range cat.Tools {
		if cat.Tools[i].ServerKey == key && cat.Tools[i].Name == tool {
			return &cat.Tools[i], nil
		}
	}
	if _, err := h.Probe(ctx, server, authorize); err != nil {
		return nil, err
	}
	cat, _ = readCatalog()
	for i := range cat.Tools {
		if cat.Tools[i].ServerKey == key && cat.Tools[i].Name == tool {
			return &cat.Tools[i], nil
		}
	}
	return nil, errors.New("MCP tool not found")
}
func (h *Hub) Call(ctx context.Context, server, tool string, args map[string]any, authorize bool) (map[string]any, error) {
	config, err := h.resolve(server)
	if err != nil {
		return nil, err
	}
	info, err := h.ToolInfo(ctx, server, tool, authorize)
	if err != nil {
		return nil, err
	}
	session, err := h.connect(ctx, config, authorize)
	if err != nil {
		return nil, err
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: info.Name, Arguments: args})
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	if connected := h.sessions[server]; connected != nil {
		connected.lastUsedAt = time.Now().UnixMilli()
	}
	h.mu.Unlock()
	return map[string]any{"server": server, "tool": tool, "readOnlyHint": func() bool {
		if ann, ok := info.Annotations.(*mcp.ToolAnnotations); ok {
			return ann.ReadOnlyHint
		}
		return false
	}(), "result": result}, nil
}
func (h *Hub) Close() {
	h.once.Do(func() {
		close(h.close)
		h.mu.Lock()
		names := []string{}
		for name := range h.sessions {
			names = append(names, name)
		}
		h.mu.Unlock()
		for _, name := range names {
			_ = h.disconnect(name)
		}
	})
}
func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}
