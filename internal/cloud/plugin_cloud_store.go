package cloud

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/plugins"

	"github.com/jackc/pgx/v5"
)

const PluginCloudDeviceID = "cloud"

const pluginCloudMigrationSQL = `
CREATE TABLE codelocal_plugin_cloud_secrets (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 plugin_id TEXT NOT NULL, nonce BYTEA NOT NULL, ciphertext BYTEA NOT NULL,
 PRIMARY KEY(user_id,plugin_id)
);
CREATE TABLE codelocal_plugin_approvals (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 session_id TEXT NOT NULL, plugin_id TEXT NOT NULL, device_id TEXT NOT NULL,
 connection_version BIGINT NOT NULL, tool TEXT NOT NULL,
 nonce BYTEA NOT NULL, ciphertext BYTEA NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','done','error','denied')),
 expires_at BIGINT NOT NULL, created_at BIGINT NOT NULL
);
CREATE INDEX idx_plugin_approvals_owner ON codelocal_plugin_approvals(user_id,expires_at);
CREATE TABLE codelocal_plugin_oauth_flows (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT NOT NULL, plugin_id TEXT NOT NULL,
 nonce BYTEA NOT NULL, ciphertext BYTEA NOT NULL, expires_at BIGINT NOT NULL
);
`

type PluginCloudCredential struct {
	Kind          string `json:"kind"`
	Endpoint      string `json:"endpoint"`
	Token         string `json:"token,omitempty"`
	RefreshToken  string `json:"refreshToken,omitempty"`
	TokenEndpoint string `json:"tokenEndpoint,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	Issuer        string `json:"issuer,omitempty"`
	ExpiresAt     int64  `json:"expiresAt,omitempty"`
}

func sealPluginValue(value any, binding string) ([]byte, []byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	aead, err := runtimeSecretAEAD()
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, data, []byte(binding)), nil
}
func openPluginValue(nonce, ciphertext []byte, binding string, out any) error {
	aead, err := runtimeSecretAEAD()
	if err != nil {
		return err
	}
	if len(nonce) != aead.NonceSize() {
		return errors.New("invalid plugin credential nonce")
	}
	data, err := aead.Open(nil, nonce, ciphertext, []byte(binding))
	if err != nil {
		return errors.New("plugin credential authentication failed")
	}
	return json.Unmarshal(data, out)
}
func cloudSecretBinding(user, plugin string) string {
	return "plugin-cloud-v1\x00" + user + "\x00" + plugin
}

func (s *Store) PluginCloudCredential(ctx context.Context, user, plugin string) (PluginCloudCredential, error) {
	var nonce, ciphertext []byte
	var out PluginCloudCredential
	if s == nil || s.DB == nil || user == "" || plugin == "" {
		return out, errors.New("plugin store unavailable")
	}
	err := s.DB.QueryRow(ctx, `SELECT nonce,ciphertext FROM codelocal_plugin_cloud_secrets WHERE user_id=$1 AND plugin_id=$2`, user, plugin).Scan(&nonce, &ciphertext)
	if err != nil {
		return out, err
	}
	err = openPluginValue(nonce, ciphertext, cloudSecretBinding(user, plugin), &out)
	return out, err
}

// SavePluginCloudConnection atomically replaces configuration and its secret.
// Cloud credentials never enter the device runtime settings inheritance chain.
func (s *Store) SavePluginCloudConnection(ctx context.Context, c PluginConnection, credential PluginCloudCredential) (PluginConnection, error) {
	c.DeviceID, c.WorkspaceKey = PluginCloudDeviceID, PluginCloudDeviceID
	c.CredentialRef = ""
	credential.Endpoint = c.Endpoint
	c, err := normalizePluginConnection(c)
	if err != nil {
		return c, err
	}
	nonce, ciphertext, err := sealPluginValue(credential, cloudSecretBinding(c.UserID, c.PluginID))
	if err != nil {
		return c, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	// Includes default-installed system plugins, whose catalog entry alone is not a DB installation.
	entry, exists := plugins.FindBuiltin(c.PluginID)
	if !exists {
		return c, errors.New("plugin is not in the catalog")
	}
	installation, err := NewPluginInstallation(c.UserID, entry.Manifest)
	if err != nil {
		return c, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO codelocal_plugin_installations(user_id,plugin_id,version,manifest_hash,state,installed_at,updated_at)
VALUES($1,$2,$3,$4,'installed',$5,$5) ON CONFLICT DO NOTHING`, c.UserID, c.PluginID, installation.Version, installation.ManifestHash, c.UpdatedAt)
	if err != nil {
		return c, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO codelocal_plugin_connections(user_id,plugin_id,device_id,workspace_key,server_name,endpoint,credential_ref,state,tool_count,last_error,connected_at,updated_at)
VALUES($1,$2,'cloud','cloud',$3,$4,'',$5,$6,$7,$8,$9)
ON CONFLICT(user_id,plugin_id,device_id) DO UPDATE SET server_name=EXCLUDED.server_name,endpoint=EXCLUDED.endpoint,credential_ref='',state=EXCLUDED.state,tool_count=EXCLUDED.tool_count,last_error=EXCLUDED.last_error,connected_at=EXCLUDED.connected_at,updated_at=GREATEST(EXCLUDED.updated_at,codelocal_plugin_connections.updated_at+1)
RETURNING updated_at`, c.UserID, c.PluginID, c.ServerName, c.Endpoint, c.State, c.ToolCount, c.LastError, c.ConnectedAt, c.UpdatedAt).Scan(&c.UpdatedAt)
	if err != nil {
		return c, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO codelocal_plugin_cloud_secrets(user_id,plugin_id,nonce,ciphertext) VALUES($1,$2,$3,$4)
ON CONFLICT(user_id,plugin_id) DO UPDATE SET nonce=EXCLUDED.nonce,ciphertext=EXCLUDED.ciphertext`, c.UserID, c.PluginID, nonce, ciphertext)
	if err != nil {
		return c, err
	}
	_, err = tx.Exec(ctx, `DELETE FROM codelocal_plugin_approvals WHERE user_id=$1 AND plugin_id=$2 AND device_id='cloud' AND status='pending'`, c.UserID, c.PluginID)
	if err != nil {
		return c, err
	}
	if err = tx.Commit(ctx); err != nil {
		return c, err
	}
	stored, _, err := s.PluginConnectionByDevice(ctx, c.UserID, c.PluginID, PluginCloudDeviceID)
	return stored, err
}

func (s *Store) DeletePluginCloudConnection(ctx context.Context, user, plugin string) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM codelocal_plugin_connections WHERE user_id=$1 AND plugin_id=$2 AND device_id='cloud'`, user, plugin)
	if err != nil {
		return false, err
	}
	for _, query := range []string{
		`DELETE FROM codelocal_plugin_cloud_secrets WHERE user_id=$1 AND plugin_id=$2`,
		`DELETE FROM codelocal_plugin_approvals WHERE user_id=$1 AND plugin_id=$2 AND device_id='cloud' AND status='pending'`,
		`DELETE FROM codelocal_runtime_secrets WHERE user_id=$1 AND scope='workspace' AND device_id='cloud' AND workspace_id='cloud' AND name=$2`,
	} {
		key := plugin
		if query == `DELETE FROM codelocal_runtime_secrets WHERE user_id=$1 AND scope='workspace' AND device_id='cloud' AND workspace_id='cloud' AND name=$2` {
			key = plugins.ManagedCredentialReference(plugin)
		}
		if _, err = tx.Exec(ctx, query, user, key); err != nil {
			return false, err
		}
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

type PluginApproval struct {
	ID        string         `json:"id"`
	UserID    string         `json:"-"`
	SessionID string         `json:"-"`
	PluginID  string         `json:"plugin"`
	DeviceID  string         `json:"deviceId"`
	Version   int64          `json:"version"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	ThreadID  string         `json:"threadId,omitempty"`
	ExpiresAt int64          `json:"expiresAt"`
}

func (s *Store) CreatePluginApproval(ctx context.Context, a PluginApproval) (PluginApproval, error) {
	if a.UserID == "" || a.SessionID == "" || a.PluginID == "" || a.Tool == "" {
		return a, errors.New("invalid approval identity")
	}
	a.ID = RandomHex(24)
	now := time.Now().UnixMilli()
	a.ExpiresAt = now + 300_000
	nonce, ciphertext, err := sealPluginValue(a, "plugin-approval\x00"+a.UserID+"\x00"+a.ID)
	if err != nil {
		return a, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM codelocal_users WHERE id=$1 FOR UPDATE`, a.UserID); err != nil {
		return a, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM codelocal_plugin_approvals WHERE user_id=$1 AND expires_at<$2`, a.UserID, now-86400000); err != nil {
		return a, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO codelocal_plugin_approvals(id,user_id,session_id,plugin_id,device_id,connection_version,tool,nonce,ciphertext,expires_at,created_at)
SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 WHERE (SELECT count(*) FROM codelocal_plugin_approvals WHERE user_id=$2 AND expires_at>$11 AND status='pending')<32`, a.ID, a.UserID, a.SessionID, a.PluginID, a.DeviceID, a.Version, a.Tool, nonce, ciphertext, a.ExpiresAt, now)
	if err == nil && tag.RowsAffected() != 1 {
		return a, errors.New("too many pending plugin approvals")
	}
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}

// TakePluginApproval claims one exact operation before dispatch. A crash or
// timeout after this point must never make it executable for a second time.
func (s *Store) TakePluginApproval(ctx context.Context, user, session, id string, deny bool) (PluginApproval, error) {
	var a PluginApproval
	var nonce, ciphertext []byte
	status := "running"
	if deny {
		status = "denied"
	}
	err := s.DB.QueryRow(ctx, `UPDATE codelocal_plugin_approvals SET status=$4 WHERE id=$1 AND user_id=$2 AND session_id=$3 AND status='pending' AND expires_at>$5 RETURNING nonce,ciphertext`, id, user, session, status, time.Now().UnixMilli()).Scan(&nonce, &ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, errors.New("approval expired, already used or unavailable")
	}
	if err != nil {
		return a, err
	}
	err = openPluginValue(nonce, ciphertext, "plugin-approval\x00"+user+"\x00"+id, &a)
	return a, err
}

func (s *Store) SavePluginOAuthFlow(ctx context.Context, id, user, session, plugin string, flow any) error {
	nonce, ciphertext, err := sealPluginValue(flow, "plugin-oauth\x00"+user+"\x00"+id)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO codelocal_plugin_oauth_flows(id,user_id,session_id,plugin_id,nonce,ciphertext,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, user, session, plugin, nonce, ciphertext, time.Now().Add(10*time.Minute).UnixMilli())
	return err
}
func (s *Store) TakePluginOAuthFlow(ctx context.Context, id, user, session string, out any) (string, error) {
	var plugin string
	var nonce, ciphertext []byte
	err := s.DB.QueryRow(ctx, `DELETE FROM codelocal_plugin_oauth_flows WHERE id=$1 AND user_id=$2 AND session_id=$3 AND expires_at>$4 RETURNING plugin_id,nonce,ciphertext`, id, user, session, time.Now().UnixMilli()).Scan(&plugin, &nonce, &ciphertext)
	if err != nil {
		return "", err
	}
	err = openPluginValue(nonce, ciphertext, "plugin-oauth\x00"+user+"\x00"+id, out)
	return plugin, err
}

// RefreshPluginCloudCredential serializes token rotation across Cloud replicas.
func (s *Store) RefreshPluginCloudCredential(ctx context.Context, user, plugin string, refresh func(PluginCloudCredential) (PluginCloudCredential, error)) (PluginCloudCredential, error) {
	var current PluginCloudCredential
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return current, err
	}
	defer tx.Rollback(ctx)
	var nonce, ciphertext []byte
	err = tx.QueryRow(ctx, `SELECT nonce,ciphertext FROM codelocal_plugin_cloud_secrets WHERE user_id=$1 AND plugin_id=$2 FOR UPDATE`, user, plugin).Scan(&nonce, &ciphertext)
	if err != nil {
		return current, err
	}
	if err = openPluginValue(nonce, ciphertext, cloudSecretBinding(user, plugin), &current); err != nil {
		return current, err
	}
	if current.Kind == "oauth" && current.ExpiresAt > 0 && current.ExpiresAt <= time.Now().Add(time.Minute).Unix() {
		current, err = refresh(current)
		if err != nil {
			return current, err
		}
		nonce, ciphertext, err = sealPluginValue(current, cloudSecretBinding(user, plugin))
		if err != nil {
			return current, err
		}
		_, err = tx.Exec(ctx, `UPDATE codelocal_plugin_cloud_secrets SET nonce=$3,ciphertext=$4 WHERE user_id=$1 AND plugin_id=$2`, user, plugin, nonce, ciphertext)
		if err != nil {
			return current, err
		}
	}
	return current, tx.Commit(ctx)
}

func (s *Store) PluginApproval(ctx context.Context, user, session, id string) (PluginApproval, error) {
	var a PluginApproval
	var nonce, ciphertext []byte
	err := s.DB.QueryRow(ctx, `SELECT nonce,ciphertext FROM codelocal_plugin_approvals WHERE id=$1 AND user_id=$2 AND session_id=$3 AND status='pending' AND expires_at>$4`, id, user, session, time.Now().UnixMilli()).Scan(&nonce, &ciphertext)
	if err != nil {
		return a, err
	}
	err = openPluginValue(nonce, ciphertext, "plugin-approval\x00"+user+"\x00"+id, &a)
	return a, err
}

// WithPluginCloudConnection pins the approved configuration through dispatch.
// Reconnect/disconnect wait on this row, so a token cannot silently change targets.
func (s *Store) WithPluginCloudConnection(ctx context.Context, user, plugin string, version int64, run func(PluginConnection) error) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	c, err := scanPluginConnection(tx.QueryRow(ctx, `SELECT workspace_key,server_name,endpoint,credential_ref,state,tool_count,last_error,connected_at,updated_at FROM codelocal_plugin_connections WHERE user_id=$1 AND plugin_id=$2 AND device_id='cloud' FOR UPDATE`, user, plugin), user, plugin, "cloud")
	if err != nil || c.UpdatedAt != version {
		return errors.New("connection changed; request a new approval")
	}
	if err = run(c); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CompletePluginApproval(ctx context.Context, user, id, status string) error {
	if status != "done" && status != "error" {
		return errors.New("invalid approval completion")
	}
	tag, err := s.DB.Exec(ctx, `UPDATE codelocal_plugin_approvals SET status=$3 WHERE id=$1 AND user_id=$2 AND status='running'`, id, user, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("approval completion unavailable")
	}
	return nil
}
