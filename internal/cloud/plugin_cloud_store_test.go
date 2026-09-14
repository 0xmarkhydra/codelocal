package cloud

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPluginCredentialAAD(t *testing.T) {
	t.Setenv("CODELOCAL_SECRET_ENCRYPTION_KEY", "isolated-test-secret-more-than-32-characters")
	credential := PluginCloudCredential{Kind: "bearer", Token: "secret"}
	nonce, ciphertext, err := sealPluginValue(credential, cloudSecretBinding("A", "penpot"))
	if err != nil {
		t.Fatal(err)
	}
	var out PluginCloudCredential
	if err := openPluginValue(nonce, ciphertext, cloudSecretBinding("B", "penpot"), &out); err == nil {
		t.Fatal("cross tenant decryption")
	}
	if err := openPluginValue(nonce, ciphertext, cloudSecretBinding("A", "github"), &out); err == nil {
		t.Fatal("cross plugin decryption")
	}
	if err := openPluginValue(nonce, ciphertext, cloudSecretBinding("A", "penpot"), &out); err != nil || out.Token != "secret" {
		t.Fatal("credential roundtrip", err)
	}
}

func TestPluginCloudPersistenceAndApprovalIntegration(t *testing.T) {
	dsn := os.Getenv("CODELOCAL_MCP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CODELOCAL_MCP_TEST_DATABASE_URL to isolated local PostgreSQL")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Host != "127.0.0.1" && config.ConnConfig.Host != "localhost" {
		t.Fatal("test requires local PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "plugin_test_" + RandomHex(8)
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Store{DB: pool}
	t.Setenv("CODELOCAL_SECRET_ENCRYPTION_KEY", "isolated-test-secret-more-than-32-characters")
	_, err = pool.Exec(ctx, `CREATE TABLE codelocal_users(id TEXT PRIMARY KEY); INSERT INTO codelocal_users VALUES('A'),('B');
CREATE TABLE codelocal_runtime_secrets(user_id text,scope text,device_id text,workspace_id text,name text);`+pluginInstallationsMigrationSQL+pluginConnectionsMigrationSQL+pluginCloudMigrationSQL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.SavePluginCloudConnection(ctx, PluginConnection{UserID: "A", PluginID: "penpot", ServerName: "plugin-penpot", Endpoint: "https://example.com/mcp", State: PluginConnectionReady, ToolCount: 3}, PluginCloudCredential{Kind: "bearer", Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Store{DB: pool}
	cred, err := restarted.PluginCloudCredential(ctx, "A", "penpot")
	if err != nil || cred.Token != "secret" {
		t.Fatal("restart lost auth", err)
	}
	if _, err = restarted.PluginCloudCredential(ctx, "B", "penpot"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross user auth", err)
	}
	approval, err := s.CreatePluginApproval(ctx, PluginApproval{UserID: "A", SessionID: "session-A", PluginID: "penpot", DeviceID: "cloud", Version: c.UpdatedAt, Tool: "execute_code", Arguments: map[string]any{"code": "create"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.TakePluginApproval(ctx, "B", "session-A", approval.ID, false); err == nil {
		t.Fatal("cross user approval")
	}
	if _, err = s.TakePluginApproval(ctx, "A", "other-session", approval.ID, false); err == nil {
		t.Fatal("cross session approval")
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := s.TakePluginApproval(ctx, "A", "session-A", approval.ID, false)
			if err == nil {
				wins.Add(1)
				if a.Version != c.UpdatedAt || a.Arguments["code"] != "create" {
					t.Error("approval action changed")
				}
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("approval dispatched %d times", wins.Load())
	}
	if err := s.CompletePluginApproval(ctx, "A", approval.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakePluginApproval(ctx, "A", "session-A", approval.ID, false); err == nil {
		t.Fatal("completed approval was reusable")
	}
	if removed, err := s.DeletePluginCloudConnection(ctx, "A", "penpot"); err != nil || !removed {
		t.Fatal("disconnect", err)
	}
	if _, err = s.PluginCloudCredential(ctx, "A", "penpot"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("disconnect kept secret", err)
	}
}
