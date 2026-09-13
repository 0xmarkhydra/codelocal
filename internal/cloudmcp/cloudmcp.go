// Package cloudmcp hosts the Cloud-side MCP client used by Plugin connections
// with the "cloud" execution target. Unlike internal/mcphub, which is bound to
// the local runtime's filesystem registry, sessions live in process and are
// keyed by user so one account can never reach another account's connection.
// Every outbound dial passes an SSRF guard: HTTPS only, and private, loopback,
// link-local and carrier-NAT addresses are refused both at URL validation time
// and again at dial time so DNS rebinding cannot bypass the URL check.
package cloudmcp

import (
	"context"
	"crypto/tls"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxConnectionsPerUser = 8
	maxTotalConnections   = 512
	connectTimeout        = 30 * time.Second
	callTimeout           = 45 * time.Second
)

// ToolSummary is the bounded tool metadata the Cloud keeps per connection. It
// is also the approval surface: only tools marked read-only by the MCP server
// may execute through a cloud connection (device connections keep the runtime
// approval engine).
type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ReadOnly    bool   `json:"readOnly"`
}

type ConnectResult struct {
	Configured bool
	Connected  bool
	ServerName string
	ToolCount  int
	Tools      []ToolSummary
	Error      string
}

type session struct {
	serverName string
	endpoint   string
	client     *mcp.ClientSession
	tools      []ToolSummary
}

// Manager owns one cloud MCP session per (user, plugin). One cloud connection
// per plugin mirrors the store's ON CONFLICT (user_id, plugin_id, device_id)
// with the reserved "cloud" device sentinel.
type Manager struct {
	mu    sync.Mutex
	users map[string]map[string]*session
	total int
}

func NewManager() *Manager {
	return &Manager{users: map[string]map[string]*session{}}
}

// ServerName mirrors the local runtime's canonical plugin server naming so the
// same plugin resolves to the same stable identifier on either target.
func ServerName(pluginID string) (string, error) {
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

// endpointAllowed validates the cloud endpoint URL. HTTPS only, no embedded
// credentials, no fragment, and IP literals must already pass the deny list.
func endpointAllowed(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("cloud MCP endpoint must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("cloud MCP endpoint cannot include credentials or a fragment")
	}
	if parsed.Scheme != "https" {
		return "", errors.New("cloud MCP endpoint must use HTTPS")
	}
	host := strings.Trim(strings.ToLower(parsed.Hostname()), "[]")
	if host == "localhost" {
		return "", errors.New("cloud MCP endpoint cannot target localhost")
	}
	if ip := net.ParseIP(host); ip != nil && !ipAllowed(ip) {
		return "", errors.New("cloud MCP endpoint cannot target private or link-local addresses")
	}
	return parsed.String(), nil
}

func ipAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.Equal(net.IPv4bcast) {
		return false
	}
	// Carrier-grade NAT (100.64.0.0/10) is unreachable from the public
	// internet but reachable inside hosting networks; deny it too.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] < 128 {
		return false
	}
	return true
}

// dialControl re-checks the resolved IP at dial time. This is the rebinding
// defense: a hostname may pass URL validation and still resolve privately.
// It is a variable only so the TLS loopback test server can dial itself; the
// URL-level guard above is never bypassed.
var dialControl = func(_ string, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("cloud MCP dial rejected invalid address: %w", err)
	}
	ip := net.ParseIP(host)
	if !ipAllowed(ip) {
		return errors.New("cloud MCP dial rejected private or link-local address")
	}
	return nil
}

// baseTLSConfig is nil in production, meaning system roots. Tests point it at
// the loopback test server's certificate.
var baseTLSConfig *tls.Config

func newGuardedTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: dialControl}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
		Proxy:                 nil,
		TLSClientConfig:       baseTLSConfig,
	}
}

// authTransport injects the connection bearer on requests to the connection
// host only, and never on redirects to a different host.
type authTransport struct {
	base   http.RoundTripper
	scheme string
	host   string
	bearer string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req
	if req.URL.Scheme == t.scheme && strings.EqualFold(req.URL.Host, t.host) {
		clone := req.Clone(req.Context())
		clone.Header.Set("Authorization", "Bearer "+t.bearer)
		out = clone
	}
	return t.base.RoundTrip(out)
}

func newSessionClient(endpoint string, bearer string) *http.Client {
	parsed, _ := url.Parse(endpoint)
	return &http.Client{
		Timeout: 0, // streamable transport holds long-lived reads; calls use ctx deadlines
		Transport: &authTransport{
			base:   newGuardedTransport(),
			scheme: parsed.Scheme,
			host:   parsed.Host,
			bearer: bearer,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return errors.New("cloud MCP endpoint redirected too many times")
			}
			if _, err := endpointURLGuard(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

// endpointURLGuard is the URL-level SSRF check applied on Connect and on every
// redirect. It is a variable only so the TLS loopback test server can pass;
// production behavior is the strict endpointAllowed.
var endpointURLGuard = endpointAllowed

func (m *Manager) get(userID, pluginID string) (*session, bool) {
	sessions, ok := m.users[userID]
	if !ok {
		return nil, false
	}
	s, ok := sessions[pluginID]
	return s, ok
}

// Connect establishes (or replaces) the user's cloud session for a plugin and
// probes its tool list. On probe failure the result carries an Error so the
// caller can persist a failed connection row, mirroring the local runtime.
func (m *Manager) Connect(ctx context.Context, userID, pluginID, endpoint, bearer string) (ConnectResult, error) {
	result := ConnectResult{}
	normalized, err := endpointURLGuard(endpoint)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(bearer) == "" {
		return result, errors.New("cloud MCP connections require a bearer token")
	}
	serverName, err := ServerName(pluginID)
	if err != nil {
		return result, err
	}

	m.mu.Lock()
	sessions, ok := m.users[userID]
	if ok && len(sessions) >= maxConnectionsPerUser {
		if _, exists := sessions[pluginID]; !exists {
			m.mu.Unlock()
			return result, errors.New("cloud MCP connection limit reached for this account")
		}
	}
	if m.total >= maxTotalConnections {
		m.mu.Unlock()
		return result, errors.New("cloud MCP connection limit reached")
	}
	if existing, exists := m.get(userID, pluginID); exists {
		_ = existing.client.Close()
		delete(sessions, pluginID)
		m.total--
	}
	m.mu.Unlock()

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-cloud", Version: "2.0.0"}, nil)
	clientSession, err := client.Connect(connectCtx, &mcp.StreamableClientTransport{
		Endpoint:   normalized,
		HTTPClient: newSessionClient(normalized, bearer),
	}, nil)
	if err != nil {
		return result, fmt.Errorf("cloud MCP connect failed: %w", err)
	}
	tools := []ToolSummary{}
	listed, listErr := clientSession.ListTools(connectCtx, nil)
	if listErr != nil {
		_ = clientSession.Close()
		result.Configured = true
		result.ServerName = serverName
		result.Error = listErr.Error()
		return result, nil
	}
	for _, tool := range listed.Tools {
		readOnly := false
		if tool.Annotations != nil {
			readOnly = tool.Annotations.ReadOnlyHint
		}
		tools = append(tools, ToolSummary{Name: tool.Name, Description: tool.Description, ReadOnly: readOnly})
	}
	result = ConnectResult{
		Configured: true,
		Connected:  true,
		ServerName: serverName,
		ToolCount:  len(tools),
		Tools:      tools,
	}
	m.mu.Lock()
	if m.users[userID] == nil {
		m.users[userID] = map[string]*session{}
	}
	m.users[userID][pluginID] = &session{serverName: serverName, endpoint: normalized, client: clientSession, tools: tools}
	m.total++
	m.mu.Unlock()
	return result, nil
}

// ListTools returns the probed tool metadata for the user's cloud connection.
func (m *Manager) ListTools(userID, pluginID string) ([]ToolSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.get(userID, pluginID)
	if !ok {
		return nil, errors.New("cloud MCP connection not found")
	}
	return append([]ToolSummary(nil), s.tools...), nil
}

// ToolInfo reports whether the named tool exists on the connection.
func (m *Manager) ToolInfo(userID, pluginID, tool string) (ToolSummary, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.get(userID, pluginID)
	if !ok {
		return ToolSummary{}, false, errors.New("cloud MCP connection not found")
	}
	for _, summary := range s.tools {
		if summary.Name == tool {
			return summary, true, nil
		}
	}
	return ToolSummary{}, false, nil
}

// Call executes a tool on the user's cloud session. Only read-only tools are
// permitted: cloud connections have no runtime approval engine, so mutating
// tools fail closed here and must run through a device connection instead.
func (m *Manager) Call(ctx context.Context, userID, pluginID, tool string, args map[string]any) (map[string]any, error) {
	m.mu.Lock()
	s, ok := m.get(userID, pluginID)
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("cloud MCP connection not found")
	}
	summary, exists, err := m.ToolInfo(userID, pluginID, tool)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("tool %q is not available on cloud connection %s", tool, pluginID)
	}
	if !summary.ReadOnly {
		return map[string]any{
			"blocked": true,
			"error":   "cloud tools that can change external data require a device connection",
		}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	result, err := s.client.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Remove closes and forgets the user's cloud session for a plugin. It reports
// whether a session existed so callers can log no-op disconnects.
func (m *Manager) Remove(userID, pluginID string) bool {
	m.mu.Lock()
	sessions, ok := m.users[userID]
	if !ok {
		m.mu.Unlock()
		return false
	}
	s, ok := sessions[pluginID]
	if ok {
		delete(sessions, pluginID)
		m.total--
		if len(sessions) == 0 {
			delete(m.users, userID)
		}
	}
	m.mu.Unlock()
	if ok && s != nil && s.client != nil {
		_ = s.client.Close()
	}
	return ok
}

// Close tears down every session; used on server shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	users := m.users
	m.users = map[string]map[string]*session{}
	m.total = 0
	m.mu.Unlock()
	for _, sessions := range users {
		for _, s := range sessions {
			if s != nil && s.client != nil {
				_ = s.client.Close()
			}
		}
	}
}
