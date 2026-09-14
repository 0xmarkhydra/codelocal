package penpot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/mcphub"
	"github.com/0xmarkhydra/codelocal/internal/oauth"
	"github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func penpotTestStore(t *testing.T) *cloud.Store {
	t.Helper()
	dsn := os.Getenv("CODELOCAL_PENPOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CODELOCAL_PENPOT_TEST_DATABASE_URL to an isolated local PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Host != "localhost" && cfg.ConnConfig.Host != "127.0.0.1" {
		t.Fatal("integration fixture requires local PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := "penpot_test_" + cloud.RandomHex(8)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `
CREATE TABLE codelocal_users (
 id text PRIMARY KEY, email text, password_hash text DEFAULT '', password_salt text DEFAULT '',
 password_changed_at bigint DEFAULT 0, security_version bigint DEFAULT 1,
 referral_code text DEFAULT '', referred_by_code text, created_at bigint DEFAULT 0);
CREATE TABLE codelocal_devices (
 credential_id text PRIMARY KEY, user_id text, device_id text, device_name text DEFAULT '',
 public_key text, secret_hash text DEFAULT '', created_at bigint DEFAULT 0, last_seen_at bigint DEFAULT 0, revoked_at bigint);
CREATE TABLE codelocal_workspaces (
 user_id text, device_id text, workspace_id text, workspace_name text DEFAULT '', project_root text,
 protocol_version integer DEFAULT 2, capabilities jsonb, created_at bigint DEFAULT 0, last_seen_at bigint DEFAULT 0);
CREATE TABLE codelocal_workspace_projects (
 user_id text, device_id text, workspace_id text, project_id text, source text, confidence double precision);
CREATE TABLE codelocal_projects (user_id text, project_id text, name text);
CREATE TABLE codelocal_plugin_cloud_secrets(user_id text,plugin_id text,nonce bytea,ciphertext bytea,PRIMARY KEY(user_id,plugin_id));
CREATE TABLE codelocal_plugin_installations(user_id text,plugin_id text,version text,manifest_hash text,state text,installed_at bigint,updated_at bigint,PRIMARY KEY(user_id,plugin_id));
CREATE TABLE codelocal_plugin_connections(user_id text,plugin_id text,device_id text,workspace_key text,server_name text,endpoint text,credential_ref text,state text,tool_count integer,last_error text,connected_at bigint,updated_at bigint,PRIMARY KEY(user_id,plugin_id,device_id));
CREATE TABLE codelocal_plugin_approvals(user_id text,plugin_id text,device_id text,status text);
CREATE TABLE codelocal_runtime_secrets (
 user_id text, scope text, device_id text, workspace_id text, name text, nonce bytea, ciphertext bytea, updated_at bigint,
 PRIMARY KEY(user_id,scope,device_id,workspace_id,name));
INSERT INTO codelocal_users(id,email) VALUES ('A','a@example.test'),('B','b@example.test');
INSERT INTO codelocal_devices(credential_id,user_id,device_id) VALUES ('credential-A','A','device'),('credential-B','B','device');
INSERT INTO codelocal_workspaces(user_id,device_id,workspace_id,capabilities)
 VALUES ('A','device','workspace','{}'),('B','device','workspace','{}');
`)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCP_AUTH_SECRET", "isolated-test-secret-not-production")
	return &cloud.Store{DB: pool}
}

func TestPenpotSignedRuntimeToAdapterIntegration(t *testing.T) {
	store := penpotTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ref := plugins.ManagedCredentialReference("penpot")
	for _, user := range []string{"A", "B"} {
		if err := store.PutRuntimeSecret(ctx, user, cloud.RuntimeScopeWorkspace, "device", "workspace", ref, "native-"+user); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := oauth.New(store, nil, "https://codelocal.test", "isolated-signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	var nativeExpired atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if nativeExpired.Load() {
			_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000000","fullname":"Anonymous User"}`))
			return
		}
		user := strings.TrimPrefix(r.Header.Get("Authorization"), "Token native-")
		email := strings.ToLower(user) + "@example.test"
		_ = json.NewEncoder(w).Encode(profile{ID: user, Email: email, Active: true})
	}))
	defer backend.Close()
	upstream := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		nativeKey := r.URL.Query().Get("userToken")
		server := mcp.NewServer(&mcp.Implementation{Name: "penpot-fixture", Version: "2.17.0"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "execute_code"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]string, error) {
			return nil, map[string]string{"fixtureOwner": strings.TrimPrefix(nativeKey, "native-")}, nil
		})
		return server
	}, nil))
	defer upstream.Close()
	adapter, err := New(auth, backend.URL, upstream.URL, "ws://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(http.HandlerFunc(adapter.ServeMCP))
	defer endpoint.Close()
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", endpoint.URL)
	grantA, err := auth.IssuePenpotGrant(ctx, "A", "device", "workspace", "native-A")
	if err != nil {
		t.Fatal(err)
	}
	var current atomic.Value
	current.Store(grantA)
	hub, err := mcphub.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	hub.SetSecretResolver(func(string) (string, bool) { return current.Load().(string), true })
	if err := hub.SetManagedPenpotCredentialRef(ref); err != nil {
		t.Fatal(err)
	}
	call := func(user string) {
		t.Helper()
		result, err := hub.Call(ctx, "penpot", "execute_code", nil, false)
		if err != nil {
			t.Fatal(err)
		}
		value := result["result"].(*mcp.CallToolResult).StructuredContent.(map[string]any)["fixtureOwner"]
		if value != user {
			t.Fatalf("fixture returned owner %v want %s", value, user)
		}
	}
	call("A")
	grantB, err := auth.IssuePenpotGrant(ctx, "B", "device", "workspace", "native-B")
	if err != nil {
		t.Fatal(err)
	}
	current.Store(grantB)
	call("B")
	wrongOwner, err := auth.IssuePenpotGrant(ctx, "B", "device", "workspace", "native-A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.authenticate(ctx, wrongOwner); err == nil {
		t.Fatal("B used A's native key")
	}
	nativeExpired.Store(true)
	if _, err := hub.Call(ctx, "penpot", "execute_code", nil, false); err == nil {
		t.Fatal("expired native token still executed")
	}
	nativeExpired.Store(false)
	if _, err := store.DB.Exec(ctx, `UPDATE codelocal_devices SET revoked_at=1 WHERE credential_id='credential-B'`); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.authenticate(ctx, grantB); err == nil {
		t.Fatal("revoked device retained access")
	}
	if _, err := store.DB.Exec(ctx, `DELETE FROM codelocal_workspaces WHERE user_id='A' AND device_id='device' AND workspace_id='workspace'`); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.authenticate(ctx, grantA); err == nil {
		t.Fatal("revoked workspace retained access")
	}
	if _, err := store.DB.Exec(ctx, `INSERT INTO codelocal_workspaces(user_id,device_id,workspace_id,capabilities) VALUES ('A','device','workspace','{}')`); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRuntimeSecret(ctx, "A", cloud.RuntimeScopeWorkspace, "device", "workspace", ref); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.authenticate(ctx, grantA); err == nil {
		t.Fatal("disconnected integration retained access")
	}
}
