package cloudmcp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEndpointAllowedRejectsNonHTTPSAndEmbeddedCredentials(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantErr  string
	}{
		{name: "plain http", endpoint: "http://example.com/mcp", wantErr: "HTTPS"},
		{name: "localhost", endpoint: "https://localhost/mcp", wantErr: "localhost"},
		{name: "loopback literal", endpoint: "https://127.0.0.1/mcp", wantErr: "private or link-local"},
		{name: "private literal", endpoint: "https://10.1.2.3/mcp", wantErr: "private or link-local"},
		{name: "link local literal", endpoint: "https://[fe80::1]/mcp", wantErr: "private or link-local"},
		{name: "metadata literal", endpoint: "https://169.254.169.254/mcp", wantErr: "private or link-local"},
		{name: "carrier nat literal", endpoint: "https://100.64.0.1/mcp", wantErr: "private or link-local"},
		{name: "user info", endpoint: "https://user:pass@example.com/mcp", wantErr: "credentials"},
		{name: "fragment", endpoint: "https://example.com/mcp#frag", wantErr: "fragment"},
		{name: "missing host", endpoint: "https:///mcp", wantErr: "absolute"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := endpointAllowed(item.endpoint); err == nil || !strings.Contains(err.Error(), item.wantErr) {
				t.Fatalf("endpointAllowed(%q) error=%v want substring %q", item.endpoint, err, item.wantErr)
			}
		})
	}
	if _, err := endpointAllowed("https://example.com/mcp"); err != nil {
		t.Fatalf("endpointAllowed rejected a valid public endpoint: %v", err)
	}
}

func TestIPAllowedDenyMatrix(t *testing.T) {
	denied := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"0.0.0.0", "255.255.255.255", "100.64.0.1", "100.127.255.255",
		"::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
	}
	for _, value := range denied {
		if ipAllowed(net.ParseIP(value)) {
			t.Fatalf("ipAllowed(%q) = true, want false", value)
		}
	}
	allowed := []string{"1.1.1.1", "8.8.8.8", "2606:4700::1111"}
	for _, value := range allowed {
		if !ipAllowed(net.ParseIP(value)) {
			t.Fatalf("ipAllowed(%q) = false, want true", value)
		}
	}
}

func TestServerNameMatchesLocalRuntimeRule(t *testing.T) {
	name, err := ServerName("penpot")
	if err != nil || name != "plugin-penpot" {
		t.Fatalf("ServerName(penpot) = %q, %v", name, err)
	}
	if _, err := ServerName("Not Canonical"); err == nil {
		t.Fatal("ServerName accepted a non-canonical plugin id")
	}
}

// stubMCPServer serves a real MCP streamable endpoint with one read-only and
// one mutating tool, requiring the expected bearer on every request.
func stubMCPServer(t *testing.T, bearer string) *httptest.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "stub-plugin", Version: "1.0.0"}, nil)
	objectSchema := &jsonschema.Schema{Type: "object"}
	server.AddTool(&mcp.Tool{Name: "search_items", Description: "search", InputSchema: objectSchema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
		})
	server.AddTool(&mcp.Tool{Name: "send_message", Description: "mutating", InputSchema: objectSchema},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "sent"}}}, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true})
	inner := http.NewServeMux()
	inner.Handle("/mcp", handler)
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearer != "" && r.Header.Get("Authorization") != "Bearer "+bearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
	testServer := httptest.NewTLSServer(guarded)
	t.Cleanup(testServer.Close)
	return testServer
}

// allowLoopbackTestServer points the URL guard, TLS roots and dial control at
// the loopback test server. Production paths keep the strict guards.
func allowLoopbackTestServer(t *testing.T, testServer *httptest.Server) {
	t.Helper()
	originalURL, originalTLS, originalDial := endpointURLGuard, baseTLSConfig, dialControl
	endpointURLGuard = func(raw string) (string, error) { return strings.TrimRight(raw, "/"), nil }
	pool := x509.NewCertPool()
	pool.AddCert(testServer.Certificate())
	baseTLSConfig = &tls.Config{RootCAs: pool}
	dialControl = func(string, string, syscall.RawConn) error { return nil }
	t.Cleanup(func() { endpointURLGuard, baseTLSConfig, dialControl = originalURL, originalTLS, originalDial })
}

func TestManagerConnectCallRemoveLifecycle(t *testing.T) {
	const bearer = "test-bearer"
	testServer := stubMCPServer(t, bearer)
	allowLoopbackTestServer(t, testServer)
	endpoint := testServer.URL + "/mcp"

	manager := NewManager()
	t.Cleanup(manager.Close)
	ctx := context.Background()

	result, err := manager.Connect(ctx, "user-1", "penpot", endpoint, bearer)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if !result.Configured || !result.Connected || result.ServerName != "plugin-penpot" {
		t.Fatalf("unexpected connect result: %#v", result)
	}
	if result.ToolCount != 2 {
		t.Fatalf("tool count = %d want 2", result.ToolCount)
	}
	tools, err := manager.ListTools("user-1", "penpot")
	if err != nil || len(tools) != 2 {
		t.Fatalf("ListTools = %v, %v", tools, err)
	}
	for _, tool := range tools {
		want := tool.Name == "search_items"
		if tool.ReadOnly != want {
			t.Fatalf("tool %s readOnly = %v want %v", tool.Name, tool.ReadOnly, want)
		}
	}

	// Tenant isolation: another account has no session here.
	if _, err := manager.ListTools("user-2", "penpot"); err == nil {
		t.Fatal("ListTools crossed the tenant boundary")
	}

	called, err := manager.Call(ctx, "user-1", "penpot", "search_items", map[string]any{"q": "x"})
	if err != nil {
		t.Fatalf("Call read-only tool failed: %v", err)
	}
	if blocked, _ := called["blocked"].(bool); blocked {
		t.Fatalf("read-only cloud call was blocked: %#v", called)
	}

	// Mutating tools fail closed on cloud connections.
	blocked, err := manager.Call(ctx, "user-1", "penpot", "send_message", map[string]any{})
	if err != nil {
		t.Fatalf("Call mutating tool returned transport error: %v", err)
	}
	if value, _ := blocked["blocked"].(bool); !value {
		t.Fatalf("mutating cloud call was not blocked: %#v", blocked)
	}

	// A wrong bearer never establishes a session.
	if _, err := manager.Connect(ctx, "user-1", "other", endpoint, "wrong"); err == nil {
		t.Fatal("Connect with a wrong bearer succeeded")
	}

	if !manager.Remove("user-1", "penpot") {
		t.Fatal("Remove reported no session")
	}
	if manager.Remove("user-1", "penpot") {
		t.Fatal("Remove reported a session after removal")
	}
}

func TestManagerConnectEnforcesConnectionLimits(t *testing.T) {
	testServer := stubMCPServer(t, "")
	allowLoopbackTestServer(t, testServer)
	manager := NewManager()
	t.Cleanup(manager.Close)
	ctx := context.Background()
	endpoint := testServer.URL + "/mcp"
	var nameLock sync.Mutex
	names := map[int]string{}
	for index := 0; index < maxConnectionsPerUser; index++ {
		nameLock.Lock()
		names[index] = "p" + strings.Repeat("x", index)
		pluginID := names[index]
		nameLock.Unlock()
		if _, err := manager.Connect(ctx, "user-1", pluginID, endpoint, "bearer"); err != nil {
			t.Fatalf("Connect #%d failed: %v", index+1, err)
		}
	}
	if _, err := manager.Connect(ctx, "user-1", "overflow", endpoint, ""); err == nil {
		t.Fatal("per-user connection limit was not enforced")
	}
}

func TestManagerConnectRejectsPrivateEndpointBeforeDialing(t *testing.T) {
	manager := NewManager()
	t.Cleanup(manager.Close)
	// No dial override here: URL validation must reject without any network.
	if _, err := manager.Connect(context.Background(), "user-1", "penpot", "https://10.0.0.5/mcp", "token"); err == nil {
		t.Fatal("Connect accepted a private endpoint")
	}
}
