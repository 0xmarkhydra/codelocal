package mcphub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This fixture mirrors the hosted adapter's header transport, not its validation.
func penpotSessionFixture(t *testing.T) *httptest.Server {
	t.Helper()
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if r.URL.RawQuery != "" {
			t.Error("managed credential exposed in public URL")
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		server := mcp.NewServer(&mcp.Implementation{Name: "penpot-test", Version: "1"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "execute_code"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]string, error) {
			return nil, map[string]string{"owner": token}, nil
		})
		return server
	}, nil)
	return httptest.NewServer(handler)
}

func TestPenpotSessionsFollowCredentialRotationAndRevocation(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	server := penpotSessionFixture(t)
	defer server.Close()
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", server.URL)
	hub, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	var token atomic.Value
	token.Store("user-A")
	hub.SetSecretResolver(func(string) (string, bool) {
		value := token.Load().(string)
		return value, value != ""
	})
	if err := hub.SetManagedPenpotCredentialRef("PENPOT_TEST"); err != nil {
		t.Fatal(err)
	}
	call := func(owner string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := hub.Call(ctx, "penpot", "execute_code", nil, false)
		if err != nil {
			t.Fatal(err)
		}
		got := result["result"].(*mcp.CallToolResult).StructuredContent.(map[string]any)["owner"]
		if got != owner {
			t.Fatalf("session belongs to %v, want %s", got, owner)
		}
	}
	call("user-A")
	hub.mu.Lock()
	oldSession := hub.sessions["penpot"].session
	hub.mu.Unlock()
	token.Store("user-B")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := oldSession.CallTool(ctx, &mcp.CallToolParams{Name: "execute_code"}); err == nil {
		t.Fatal("old session remained usable after credential changed")
	}
	call("user-B")
	token.Store("")
	if _, err := hub.Call(context.Background(), "penpot", "execute_code", nil, false); err == nil {
		t.Fatal("revoked credential reused an authenticated session")
	}
}

func TestPenpotMissingAuthCannotUseGlobalEnvironment(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", "")
	t.Setenv("PENPOT_TEST", "global-token")
	hub, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	cfg, _ := managedPenpotConfig()
	if _, err := hub.managedPenpotQuery(cfg); err == nil {
		t.Fatal("missing auth context accepted")
	}
	if err := hub.SetManagedPenpotCredentialRef("PENPOT_TEST"); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.managedPenpotQuery(cfg); err == nil {
		t.Fatal("process-global token accepted")
	}
}

func TestPenpotRegistrationSurvivesRestartAndBackendFailure(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", server.URL)
	root := t.TempDir()
	for i := 0; i < 2; i++ {
		hub, err := New(root, nil)
		if err != nil {
			t.Fatal(err)
		}
		hub.SetSecretResolver(func(string) (string, bool) { return "test-account-token", true })
		if err := hub.SetManagedPenpotCredentialRef("PENPOT_TEST"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err = hub.Probe(ctx, "penpot", false)
		cancel()
		if err == nil {
			t.Fatal("unavailable backend reported ready")
		}
		list, err := hub.List()
		hub.Close()
		if err != nil || len(list.([]map[string]any)) != 1 || list.([]map[string]any)[0]["name"] != "penpot" {
			t.Fatalf("managed registration lost after failure/restart: %v %v", list, err)
		}
	}
}

func TestPenpotInvalidOverrideKeepsSystemRegistration(t *testing.T) {
	for _, raw := range []string{"not-a-url", "https:///mcp", "https://user:secret@example.com/mcp", "https://example.com/mcp?userToken=secret"} {
		t.Setenv("CODELOCAL_PENPOT_MCP_URL", raw)
		cfg, ok := managedPenpotConfig()
		if !ok || cfg.URL != managedPenpotURL {
			t.Fatalf("invalid override did not fall back: %q", raw)
		}
	}
}

type penpotFailingTransport struct{}

func (penpotFailingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New(r.URL.String())
}

func TestPenpotTransportDoesNotExposeQueryCredentialInErrors(t *testing.T) {
	request, _ := http.NewRequest(http.MethodPost, "https://example.com/mcp", nil)
	transport := headerTransport{base: penpotFailingTransport{}, query: map[string][]string{"userToken": {"private-token"}}}
	_, err := transport.RoundTrip(request)
	if err == nil || strings.Contains(err.Error(), "private-token") || request.URL.RawQuery != "" {
		t.Fatal("transport leaked credential or modified caller URL")
	}
}
