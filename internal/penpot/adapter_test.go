package penpot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/oauth"
	"github.com/coder/websocket"
)

func fixtureAdapter(t *testing.T, handler http.Handler) (*Adapter, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	auth, err := oauth.New(nil, nil, "https://codelocal.test", "local-test-signing-key")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(auth, server.URL, server.URL, strings.Replace(server.URL, "http:", "ws:", 1))
	if err != nil {
		t.Fatal(err)
	}
	return adapter, server
}

func TestNativeOwnerValidationExpiryAndAnonymous(t *testing.T) {
	var body atomic.Value
	body.Store(`{"id":"11111111-1111-1111-1111-111111111111","email":"a@example.test","isActive":true}`)
	adapter, _ := fixtureAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/rpc/command/get-profile" || r.Header.Get("Authorization") != "Token native-A" ||
			r.Header.Get("Cookie") != "" || r.Method != http.MethodPost || r.URL.RawQuery != "" {
			t.Error("native credential sent outside expected API boundary")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	ctx := context.Background()
	if err := adapter.ValidateOwner(ctx, "native-A", "A@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ValidateOwner(ctx, "native-A", "b@example.test"); err == nil {
		t.Fatal("User B accepted User A credential")
	}
	for _, invalid := range []string{
		`{"id":"00000000-0000-0000-0000-000000000000","fullname":"Anonymous User"}`,
		`{"id":"id","email":"a@example.test","isActive":false}`,
		`{"id":"id","email":"a@example.test","isActive":true,"isBlocked":true}`,
		`{"id":"id","email":"a@example.test","is-active":true}`,
		`{}`,
		`not-json`,
	} {
		body.Store(invalid)
		if err := adapter.ValidateOwner(ctx, "native-A", "a@example.test"); err == nil {
			t.Fatal("expired, anonymous or inactive profile accepted")
		}
	}
}

func TestMissingAuthAndDisabledIntegrationNeverReachUpstream(t *testing.T) {
	var calls atomic.Int32
	adapter, _ := fixtureAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	for _, target := range []string{"/api/v1/penpot/mcp", "/api/v1/penpot/mcp?userToken=native-A"} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader("{}"))
		response := httptest.NewRecorder()
		adapter.ServeMCP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatal("missing signed auth accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unauthenticated request reached upstream")
	}
	var disabled *Adapter
	response := httptest.NewRecorder()
	disabled.ServeMCP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal("disabled integration must report unavailable without panic")
	}
}

func testGrant(t *testing.T, user string) oauth.PenpotGrant {
	t.Helper()
	var grant oauth.PenpotGrant
	// Public shape only; this helper does not fake a signed authentication.
	raw, _ := json.Marshal(map[string]any{
		"sub": user, "device_id": "device", "workspace_id": "workspace",
		"penpot_key": "native-" + user, "exp": time.Now().Add(time.Minute).Unix(),
	})
	if err := json.Unmarshal(raw, &grant); err != nil {
		t.Fatal(err)
	}
	return grant
}

func TestAuthenticatedProxySessionBindingAndHeaderIsolation(t *testing.T) {
	var calls atomic.Int32
	adapter, _ := fixtureAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/mcp" || r.URL.Query().Get("userToken") != "native-A" ||
			r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" ||
			r.Header.Get("X-Device-Credential") != "" || r.Header.Get("X-Forwarded-User") != "" {
			t.Error("proxy crossed credential boundary")
		}
		if sid := r.Header.Get("Mcp-Session-Id"); sid != "" && sid != "upstream-A" {
			t.Error("invalid upstream session forwarded")
		}
		w.Header().Set("Mcp-Session-Id", "upstream-A")
		w.Header().Set("Set-Cookie", "upstream=private")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	grant := testGrant(t, "A")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/penpot/mcp", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer private-grant")
	request.Header.Set("Cookie", "browser=private")
	request.Header.Set("X-Device-Credential", "device-secret")
	request.Header.Set("X-Forwarded-User", "B")
	response := httptest.NewRecorder()
	adapter.serveAuthenticatedMCP(response, request, grant)
	sealed := response.Header().Get("Mcp-Session-Id")
	if response.Code != http.StatusOK || sealed == "" || sealed == "upstream-A" || response.Header().Get("Set-Cookie") != "" {
		t.Fatal("session response was not sealed")
	}
	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		for _, user := range []string{"A", "B"} {
			request := httptest.NewRequest(method, "/api/v1/penpot/mcp", nil)
			request.Header.Set("Mcp-Session-Id", sealed)
			response := httptest.NewRecorder()
			adapter.serveAuthenticatedMCP(response, request, testGrant(t, user))
			want := http.StatusUnauthorized
			if user == "A" {
				want = http.StatusOK
			}
			if response.Code != want {
				t.Fatalf("%s user %s status %d want %d", method, user, response.Code, want)
			}
		}
	}
	if calls.Load() != 4 {
		t.Fatal("cross-user session reached upstream")
	}
}

func TestNativeRedirectAndOutageFailClosed(t *testing.T) {
	var leaked atomic.Bool
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer trap.Close()
	adapter, server := fixtureAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, trap.URL, http.StatusTemporaryRedirect)
	}))
	if err := adapter.ValidateOwner(context.Background(), "private-native-key", "a@example.test"); err == nil || strings.Contains(err.Error(), "private-native-key") {
		t.Fatal("redirect was accepted or leaked credential")
	}
	if leaked.Load() {
		t.Fatal("native token forwarded across redirect")
	}
	server.Close()
	if err := adapter.ValidateOwner(context.Background(), "private-native-key", "a@example.test"); err == nil {
		t.Fatal("unavailable backend accepted")
	}
}

func TestWebSocketOwnerResponseAndRevocation(t *testing.T) {
	for _, scenario := range []string{"valid-A", "valid-B", "forged-response", "expired", "wrong-origin"} {
		t.Run(scenario, func(t *testing.T) {
			var expired atomic.Bool
			var responses atomic.Int32
			var connections atomic.Int32
			adapter, _ := fixtureAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/rpc/command/get-profile" {
					if expired.Load() {
						_, _ = w.Write([]byte(`{}`))
						return
					}
					user := strings.TrimPrefix(r.Header.Get("Authorization"), "Token native-")
					_ = json.NewEncoder(w).Encode(profile{ID: user, Email: user + "@example.test", Active: true})
					return
				}
				connections.Add(1)
				if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
					t.Error("browser credentials forwarded to native WebSocket")
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				user := strings.TrimPrefix(r.URL.Query().Get("userToken"), "native-")
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				_ = conn.Write(ctx, websocket.MessageText, []byte(`{"id":"task-`+user+`","task":"executeCode"}`))
				if _, _, err := conn.Read(ctx); err == nil {
					responses.Add(1)
				}
			}))
			public := httptest.NewServer(http.HandlerFunc(adapter.ServeWS))
			defer public.Close()
			user := "B"
			if scenario == "valid-A" {
				user = "A"
			}
			origin := "https://design.codelocal.cloud"
			if scenario == "wrong-origin" {
				origin = "https://attacker.test"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, strings.Replace(public.URL, "http:", "ws:", 1)+"?userToken=native-"+user,
				&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}, "Cookie": {"browser=private"}}})
			if scenario == "wrong-origin" {
				if err == nil || response.StatusCode != http.StatusUnauthorized || connections.Load() != 0 {
					t.Fatal("wrong Origin reached native WebSocket")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			if _, _, err := conn.Read(ctx); err != nil {
				t.Fatal(err)
			}
			taskUser := user
			if scenario == "forged-response" {
				taskUser = "A"
			}
			expired.Store(scenario == "expired")
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"id":"task-`+taskUser+`","success":true}`)); err != nil {
				t.Fatal(err)
			}
			_, _, _ = conn.Read(ctx)
			want := int32(0)
			if strings.HasPrefix(scenario, "valid-") {
				want = 1
			}
			if responses.Load() != want {
				t.Fatalf("upstream accepted %d responses; want %d", responses.Load(), want)
			}
		})
	}
}
