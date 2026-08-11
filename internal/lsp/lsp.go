package lsp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Spec struct {
	ID        string
	Languages []string
	Command   string
	Args      []string
	Exts      []string
}

var specs = []Spec{
	{ID: "typescript-language-server", Languages: []string{"typescript", "javascript"}, Command: "typescript-language-server", Args: []string{"--stdio"}, Exts: []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}},
	{ID: "pyright", Languages: []string{"python"}, Command: "pyright-langserver", Args: []string{"--stdio"}, Exts: []string{".py"}},
	{ID: "rust-analyzer", Languages: []string{"rust"}, Command: "rust-analyzer", Exts: []string{".rs"}},
	{ID: "gopls", Languages: []string{"go"}, Command: "gopls", Exts: []string{".go"}},
	{ID: "clangd", Languages: []string{"c", "cpp"}, Command: "clangd", Exts: []string{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp"}},
	{ID: "sourcekit-lsp", Languages: []string{"swift"}, Command: "sourcekit-lsp", Exts: []string{".swift"}},
	{ID: "jdtls", Languages: []string{"java"}, Command: "jdtls", Exts: []string{".java"}},
	{ID: "kotlin-language-server", Languages: []string{"kotlin"}, Command: "kotlin-language-server", Exts: []string{".kt", ".kts"}},
	{ID: "lua-language-server", Languages: []string{"lua"}, Command: "lua-language-server", Exts: []string{".lua"}},
	{ID: "zls", Languages: []string{"zig"}, Command: "zls", Exts: []string{".zig"}},
	{ID: "dart-analyzer", Languages: []string{"dart"}, Command: "dart", Args: []string{"language-server", "--protocol=lsp"}, Exts: []string{".dart"}},
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type response struct {
	result json.RawMessage
	err    error
}

type openedFile struct {
	version int
	hash    [32]byte
}

type Client struct {
	spec        Spec
	root        string
	ctx         context.Context
	cancel      context.CancelFunc
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	reader      *bufio.Reader
	writeMu     sync.Mutex
	mu          sync.Mutex
	pending     map[int64]chan response
	opened      map[string]openedFile
	diagnostics map[string][]map[string]any
	nextID      atomic.Int64
	closed      chan struct{}
	closeOnce   sync.Once
}

type Manager struct {
	root    string
	mu      sync.Mutex
	clients map[string]*Client
}

func NewManager(root string) *Manager {
	return &Manager{root: root, clients: map[string]*Client{}}
}

func specFor(path string) *Spec {
	ext := strings.ToLower(filepath.Ext(path))
	for i := range specs {
		for _, candidate := range specs[i].Exts {
			if ext == candidate {
				copy := specs[i]
				return &copy
			}
		}
	}
	return nil
}

func languageID(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".go":
		return "go"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp":
		return "cpp"
	case ".swift":
		return "swift"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".dart":
		return "dart"
	default:
		return "plaintext"
	}
}

func fileURI(path string) string {
	abs, _ := filepath.Abs(path)
	if filepath.Separator == '\\' {
		abs = filepath.ToSlash(abs)
		if !strings.HasPrefix(abs, "/") {
			abs = "/" + abs
		}
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}

func uriPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" {
		return raw
	}
	path, err := url.PathUnescape(u.Path)
	if err != nil {
		path = u.Path
	}
	if filepath.Separator == '\\' && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func (m *Manager) clientFor(ctx context.Context, path string) (*Client, error) {
	spec := specFor(path)
	if spec == nil {
		return nil, nil
	}
	command, err := exec.LookPath(spec.Command)
	if err != nil {
		return nil, nil
	}
	key := spec.ID
	m.mu.Lock()
	if client := m.clients[key]; client != nil {
		m.mu.Unlock()
		return client, nil
	}
	client, err := startClient(ctx, *spec, command, m.root)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.clients[key] = client
	m.mu.Unlock()
	return client, nil
}

func startClient(parent context.Context, spec Spec, command, root string) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, command, spec.Args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CI=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	client := &Client{spec: spec, root: root, ctx: ctx, cancel: cancel, cmd: cmd, stdin: stdin, reader: bufio.NewReaderSize(stdout, 64<<10), pending: map[int64]chan response{}, opened: map[string]openedFile{}, diagnostics: map[string][]map[string]any{}, closed: make(chan struct{})}
	go io.Copy(io.Discard, stderr)
	go client.readLoop()
	go func() {
		_ = cmd.Wait()
		client.closePending(errors.New("language server exited"))
	}()

	initCtx, initCancel := context.WithTimeout(parent, 12*time.Second)
	defer initCancel()
	_, err = client.request(initCtx, "initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(root),
		"capabilities": map[string]any{
			"workspace": map[string]any{"workspaceFolders": true, "symbol": map[string]any{"dynamicRegistration": false}},
			"textDocument": map[string]any{
				"synchronization": map[string]any{"didSave": true, "dynamicRegistration": false},
				"definition":      map[string]any{"dynamicRegistration": false, "linkSupport": true},
				"references":      map[string]any{"dynamicRegistration": false},
				"implementation":  map[string]any{"dynamicRegistration": false, "linkSupport": true},
				"documentSymbol":  map[string]any{"dynamicRegistration": false, "hierarchicalDocumentSymbolSupport": true},
				"hover":           map[string]any{"dynamicRegistration": false, "contentFormat": []string{"markdown", "plaintext"}},
			},
		},
		"workspaceFolders": []map[string]any{{"uri": fileURI(root), "name": filepath.Base(root)}},
	})
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("initialize %s: %w", spec.ID, err)
	}
	if err := client.notify("initialized", map[string]any{}); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) closePending(err error) {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		for id, waiter := range c.pending {
			delete(c.pending, id)
			select {
			case waiter <- response{err: err}:
			default:
			}
		}
		c.mu.Unlock()
	})
}

func (c *Client) readLoop() {
	for {
		payload, err := readFrame(c.reader)
		if err != nil {
			c.closePending(err)
			return
		}
		var env envelope
		if json.Unmarshal(payload, &env) != nil {
			continue
		}
		if len(env.ID) > 0 && string(env.ID) != "null" {
			id, err := strconv.ParseInt(strings.Trim(string(env.ID), `"`), 10, 64)
			if err != nil {
				continue
			}
			c.mu.Lock()
			waiter := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if waiter != nil {
				if env.Error != nil {
					waiter <- response{err: fmt.Errorf("LSP %d: %s", env.Error.Code, env.Error.Message)}
				} else {
					waiter <- response{result: env.Result}
				}
			}
			continue
		}
		if env.Method == "textDocument/publishDiagnostics" {
			var params struct {
				URI         string           `json:"uri"`
				Diagnostics []map[string]any `json:"diagnostics"`
			}
			if json.Unmarshal(env.Params, &params) == nil {
				c.mu.Lock()
				c.diagnostics[uriPath(params.URI)] = params.Diagnostics
				c.mu.Unlock()
			}
		}
	}
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	length := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
			length, err = strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
		}
	}
	if length <= 0 || length > 64<<20 {
		return nil, errors.New("invalid LSP Content-Length")
	}
	payload := make([]byte, length)
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

func (c *Client) write(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(raw))
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(raw)
	return err
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	waiter := make(chan response, 1)
	c.mu.Lock()
	c.pending[id] = waiter
	c.mu.Unlock()
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case result := <-waiter:
		return result.result, result.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("language server closed")
	}
}

func (c *Client) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) ensureOpen(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	c.mu.Lock()
	opened, exists := c.opened[absolute]
	if exists && opened.hash == hash {
		c.mu.Unlock()
		return nil
	}
	version := 1
	if exists {
		version = opened.version + 1
	}
	c.opened[absolute] = openedFile{version: version, hash: hash}
	c.mu.Unlock()
	if !exists {
		return c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": fileURI(absolute), "languageId": languageID(absolute), "version": version, "text": string(data)}})
	}
	return c.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": fileURI(absolute), "version": version}, "contentChanges": []map[string]any{{"text": string(data)}}})
}

func position(line, column int) map[string]int {
	if line < 1 {
		line = 1
	}
	if column < 1 {
		column = 1
	}
	return map[string]int{"line": line - 1, "character": column - 1}
}

func (m *Manager) requestAt(ctx context.Context, path, method string, line, column int, extra map[string]any) (json.RawMessage, string, error) {
	client, err := m.clientFor(ctx, path)
	if err != nil || client == nil {
		return nil, "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if err := client.ensureOpen(absolute); err != nil {
		return nil, "", err
	}
	params := map[string]any{"textDocument": map[string]any{"uri": fileURI(absolute)}, "position": position(line, column)}
	for key, value := range extra {
		params[key] = value
	}
	raw, err := client.request(ctx, method, params)
	return raw, client.spec.ID, err
}

func decodeAny(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func normalizeLocation(value any, provider string) []map[string]any {
	items := []any{}
	switch raw := value.(type) {
	case []any:
		items = raw
	case map[string]any:
		items = []any{raw}
	default:
		return nil
	}
	out := []map[string]any{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		uri, _ := m["uri"].(string)
		rangeValue := m["range"]
		if uri == "" {
			uri, _ = m["targetUri"].(string)
			if target, ok := m["targetSelectionRange"]; ok {
				rangeValue = target
			} else if target, ok := m["targetRange"]; ok {
				rangeValue = target
			}
		}
		entry := map[string]any{"provider": provider, "uri": uri, "path": uriPath(uri), "range": rangeValue}
		if rangeMap, ok := rangeValue.(map[string]any); ok {
			if start, ok := rangeMap["start"].(map[string]any); ok {
				if line, ok := start["line"].(float64); ok {
					entry["line"] = int(line) + 1
				}
				if column, ok := start["character"].(float64); ok {
					entry["column"] = int(column) + 1
				}
			}
		}
		out = append(out, entry)
	}
	return out
}

func (m *Manager) Definition(ctx context.Context, path string, line, column int) ([]map[string]any, error) {
	raw, provider, err := m.requestAt(ctx, path, "textDocument/definition", line, column, nil)
	if err != nil || provider == "" {
		return nil, err
	}
	return normalizeLocation(decodeAny(raw), provider), nil
}

func (m *Manager) References(ctx context.Context, path string, line, column int) ([]map[string]any, error) {
	raw, provider, err := m.requestAt(ctx, path, "textDocument/references", line, column, map[string]any{"context": map[string]any{"includeDeclaration": true}})
	if err != nil || provider == "" {
		return nil, err
	}
	return normalizeLocation(decodeAny(raw), provider), nil
}

func (m *Manager) Implementations(ctx context.Context, path string, line, column int) ([]map[string]any, error) {
	raw, provider, err := m.requestAt(ctx, path, "textDocument/implementation", line, column, nil)
	if err != nil || provider == "" {
		return nil, err
	}
	return normalizeLocation(decodeAny(raw), provider), nil
}

func (m *Manager) Hover(ctx context.Context, path string, line, column int) (map[string]any, error) {
	raw, provider, err := m.requestAt(ctx, path, "textDocument/hover", line, column, nil)
	if err != nil || provider == "" {
		return nil, err
	}
	value := decodeAny(raw)
	if value == nil {
		return nil, nil
	}
	return map[string]any{"provider": provider, "hover": value}, nil
}

func (m *Manager) DocumentSymbols(ctx context.Context, path string) ([]map[string]any, error) {
	client, err := m.clientFor(ctx, path)
	if err != nil || client == nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := client.ensureOpen(absolute); err != nil {
		return nil, err
	}
	raw, err := client.request(ctx, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": fileURI(absolute)}})
	if err != nil {
		return nil, err
	}
	values, _ := decodeAny(raw).([]any)
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if symbol, ok := value.(map[string]any); ok {
			symbol["provider"] = client.spec.ID
			symbol["path"] = absolute
			out = append(out, symbol)
		}
	}
	return out, nil
}

func (m *Manager) activeClients() []*Client {
	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	m.mu.Unlock()
	return clients
}

func workspaceSymbolsFromClients(ctx context.Context, clients []*Client, query string) []map[string]any {
	type providerResult struct {
		values []map[string]any
	}
	results := make(chan providerResult, len(clients))
	for _, client := range clients {
		client := client
		go func() {
			raw, err := client.request(ctx, "workspace/symbol", map[string]any{"query": query})
			if err != nil {
				results <- providerResult{}
				return
			}
			values, _ := decodeAny(raw).([]any)
			out := make([]map[string]any, 0, len(values))
			for _, value := range values {
				if symbol, ok := value.(map[string]any); ok {
					symbol["provider"] = client.spec.ID
					out = append(out, symbol)
				}
			}
			results <- providerResult{values: out}
		}()
	}
	out := []map[string]any{}
	for range clients {
		out = append(out, (<-results).values...)
	}
	sort.Slice(out, func(i, j int) bool {
		left := fmt.Sprint(out[i]["provider"], ":", out[i]["name"], ":", out[i]["path"])
		right := fmt.Sprint(out[j]["provider"], ":", out[j]["name"], ":", out[j]["path"])
		return left < right
	})
	return out
}

// ActiveWorkspaceSymbols never starts a language server. Context discovery uses
// it to benefit from warm providers without paying an arbitrary cold-start cost.
func (m *Manager) ActiveWorkspaceSymbols(ctx context.Context, query string) ([]map[string]any, error) {
	return workspaceSymbolsFromClients(ctx, m.activeClients(), query), nil
}

func (m *Manager) WorkspaceSymbols(ctx context.Context, query string) ([]map[string]any, error) {
	clients := m.activeClients()
	if len(clients) == 0 {
		for _, spec := range specs {
			if _, err := exec.LookPath(spec.Command); err == nil {
				client, startErr := m.clientFor(ctx, "probe"+spec.Exts[0])
				if startErr == nil && client != nil {
					clients = append(clients, client)
				}
				break
			}
		}
	}
	return workspaceSymbolsFromClients(ctx, clients, query), nil
}

func (m *Manager) Diagnostics(ctx context.Context, path string) ([]map[string]any, error) {
	client, err := m.clientFor(ctx, path)
	if err != nil || client == nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := client.ensureOpen(absolute); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(75 * time.Millisecond):
	}
	client.mu.Lock()
	values := append([]map[string]any(nil), client.diagnostics[absolute]...)
	client.mu.Unlock()
	for _, item := range values {
		item["provider"] = client.spec.ID
		item["path"] = absolute
	}
	return values, nil
}

func (m *Manager) Info() map[string]any {
	providers := make([]map[string]any, 0, len(specs))
	m.mu.Lock()
	active := map[string]bool{}
	for id := range m.clients {
		active[id] = true
	}
	m.mu.Unlock()
	for _, spec := range specs {
		_, err := exec.LookPath(spec.Command)
		providers = append(providers, map[string]any{"id": spec.ID, "languages": spec.Languages, "installed": err == nil, "active": active[spec.ID]})
	}
	return map[string]any{"providers": providers, "fallback": "go-native-structure/ripgrep", "routing": "file-extension/language-server"}
}

func (m *Manager) Close() {
	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	m.clients = map[string]*Client{}
	m.mu.Unlock()
	for _, client := range clients {
		client.Close()
	}
}

func (c *Client) Close() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, _ = c.request(shutdownCtx, "shutdown", nil)
	cancel()
	_ = c.notify("exit", nil)
	c.cancel()
	_ = c.stdin.Close()
	c.closePending(errors.New("language server closed"))
}

func (m *Manager) Available(path string) bool {
	spec := specFor(path)
	if spec == nil {
		return false
	}
	_, err := exec.LookPath(spec.Command)
	return err == nil
}

func Relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func BufferLine(text string, line int) string {
	if line < 1 {
		line = 1
	}
	scanner := bufio.NewScanner(bytes.NewBufferString(text))
	current := 0
	for scanner.Scan() {
		current++
		if current == line {
			return scanner.Text()
		}
	}
	return ""
}
