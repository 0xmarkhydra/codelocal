package cloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/mcpconfig"
	"github.com/jackc/pgx/v5"
)

type RuntimeScope string

const (
	RuntimeScopeGlobal    RuntimeScope = "global"
	RuntimeScopeDevice    RuntimeScope = "device"
	RuntimeScopeWorkspace RuntimeScope = "workspace"
	RuntimeScopeSession   RuntimeScope = "session"
)

type RuntimeSecretRef struct {
	Configured bool `json:"configured"`
}

type RuntimeSystemProject struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Source  string `json:"source,omitempty"`
	Version string `json:"version,omitempty"`
	Managed bool   `json:"managed"`
	Hidden  bool   `json:"hidden"`
	Enabled bool   `json:"enabled"`
}

type RuntimeConfigLayer struct {
	Scope          RuntimeScope                `json:"scope"`
	Values         map[string]string           `json:"values,omitempty"`
	Secrets        map[string]RuntimeSecretRef `json:"secrets,omitempty"`
	SystemProjects []RuntimeSystemProject      `json:"systemProjects,omitempty"`
	UpdatedAt      int64                       `json:"updatedAt,omitempty"`
}

type RuntimeConfigSnapshot struct {
	Values         map[string]string           `json:"values,omitempty"`
	Secrets        map[string]RuntimeSecretRef `json:"secrets,omitempty"`
	SystemProjects []RuntimeSystemProject      `json:"systemProjects,omitempty"`
	Version        int64                       `json:"version"`
	UpdatedAt      int64                       `json:"updatedAt"`
}

type RuntimeMaterializedConfig struct {
	Snapshot   RuntimeConfigSnapshot          `json:"snapshot"`
	Secrets    map[string]string              `json:"secrets,omitempty"`
	MCPServers []mcpconfig.MaterializedServer `json:"mcpServers,omitempty"`
}

const runtimeConfigMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_runtime_config (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 scope TEXT NOT NULL CHECK (scope IN ('global','device','workspace')),
 device_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '',
 config JSONB NOT NULL DEFAULT '{}'::jsonb,
 system_projects JSONB NOT NULL DEFAULT '[]'::jsonb,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,scope,device_id,workspace_id),
 CHECK (
  (scope='global' AND device_id='' AND workspace_id='') OR
  (scope='device' AND device_id<>'' AND workspace_id='') OR
  (scope='workspace' AND device_id<>'' AND workspace_id<>'')
 )
);`

const runtimeSecretsMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_runtime_secrets (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 scope TEXT NOT NULL CHECK (scope IN ('global','device','workspace')),
 device_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '',
 name TEXT NOT NULL,
 nonce BYTEA NOT NULL,
 ciphertext BYTEA NOT NULL,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,scope,device_id,workspace_id,name),
 CHECK (
  (scope='global' AND device_id='' AND workspace_id='') OR
  (scope='device' AND device_id<>'' AND workspace_id='') OR
  (scope='workspace' AND device_id<>'' AND workspace_id<>'')
 )
);`

func MergeRuntimeConfig(layers ...RuntimeConfigLayer) RuntimeConfigSnapshot {
	out := RuntimeConfigSnapshot{Values: map[string]string{}, Secrets: map[string]RuntimeSecretRef{}}
	projects := map[string]RuntimeSystemProject{}
	for _, layer := range layers {
		for key, value := range layer.Values {
			if key = strings.TrimSpace(key); key != "" {
				out.Values[key] = value
			}
		}
		for key, ref := range layer.Secrets {
			if key = strings.TrimSpace(key); key != "" {
				out.Secrets[key] = ref
			}
		}
		for _, project := range layer.SystemProjects {
			if id := strings.TrimSpace(project.ID); id != "" {
				project.ID, projects[id] = id, project
			}
		}
		if layer.UpdatedAt > out.UpdatedAt {
			out.UpdatedAt = layer.UpdatedAt
		}
	}
	ids := make([]string, 0, len(projects))
	for id := range projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out.SystemProjects = append(out.SystemProjects, projects[id])
	}
	if out.UpdatedAt == 0 {
		out.UpdatedAt = time.Now().UnixMilli()
	}
	out.Version = out.UpdatedAt
	return out
}

func ValidRuntimeEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, r := range key {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
		if letter || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func defaultRuntimeConfigLayer() RuntimeConfigLayer {
	return RuntimeConfigLayer{Scope: RuntimeScopeGlobal, SystemProjects: []RuntimeSystemProject{{
		ID: "openmontage", Name: "OpenMontage", Source: "https://github.com/calesthio/OpenMontage.git",
		Managed: true, Hidden: true, Enabled: true,
	}}}
}

func normalizeRuntimeScope(scope RuntimeScope, deviceID, workspaceID string) (string, string, string, error) {
	deviceID, workspaceID = strings.TrimSpace(deviceID), strings.TrimSpace(workspaceID)
	switch scope {
	case RuntimeScopeGlobal:
		return string(scope), "", "", nil
	case RuntimeScopeDevice:
		if deviceID != "" && workspaceID == "" {
			return string(scope), deviceID, "", nil
		}
	case RuntimeScopeWorkspace:
		if deviceID != "" && workspaceID != "" {
			return string(scope), deviceID, workspaceID, nil
		}
	}
	return "", "", "", errors.New("invalid runtime config scope")
}

func runtimeSecretKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_SECRET_ENCRYPTION_KEY"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("MCP_AUTH_SECRET"))
	}
	if len(raw) < 32 {
		return nil, errors.New("runtime secret encryption requires CODELOCAL_SECRET_ENCRYPTION_KEY or MCP_AUTH_SECRET with at least 32 characters")
	}
	sum := sha256.Sum256([]byte("codelocal-runtime-secrets-v1\x00" + raw))
	return sum[:], nil
}

func runtimeSecretAEAD() (cipher.AEAD, error) {
	key, err := runtimeSecretKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func encryptRuntimeSecret(value string) ([]byte, []byte, error) {
	aead, err := runtimeSecretAEAD()
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, []byte(value), nil), nil
}

func decryptRuntimeSecret(nonce, ciphertext []byte) (string, error) {
	aead, err := runtimeSecretAEAD()
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Store) RuntimeSettingsLayer(ctx context.Context, userID string, scope RuntimeScope, deviceID, workspaceID string) (RuntimeConfigLayer, error) {
	scopeValue, deviceID, workspaceID, err := normalizeRuntimeScope(scope, deviceID, workspaceID)
	if err != nil {
		return RuntimeConfigLayer{}, err
	}
	layer := RuntimeConfigLayer{Scope: scope, Values: map[string]string{}, Secrets: map[string]RuntimeSecretRef{}}
	var configRaw, projectsRaw []byte
	err = s.DB.QueryRow(ctx, `SELECT config,system_projects,updated_at FROM codelocal_runtime_config WHERE user_id=$1 AND scope=$2 AND device_id=$3 AND workspace_id=$4`, userID, scopeValue, deviceID, workspaceID).Scan(&configRaw, &projectsRaw, &layer.UpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return layer, err
	}
	if err == nil {
		_ = json.Unmarshal(configRaw, &layer.Values)
		_ = json.Unmarshal(projectsRaw, &layer.SystemProjects)
	}
	rows, err := s.DB.Query(ctx, `SELECT name FROM codelocal_runtime_secrets WHERE user_id=$1 AND scope=$2 AND device_id=$3 AND workspace_id=$4 ORDER BY name`, userID, scopeValue, deviceID, workspaceID)
	if err != nil {
		return layer, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return layer, err
		}
		layer.Secrets[name] = RuntimeSecretRef{Configured: true}
	}
	return layer, rows.Err()
}

func (s *Store) PutRuntimeSettingsLayer(ctx context.Context, userID string, layer RuntimeConfigLayer, deviceID, workspaceID string) error {
	scope, deviceID, workspaceID, err := normalizeRuntimeScope(layer.Scope, deviceID, workspaceID)
	if err != nil {
		return err
	}
	values, err := json.Marshal(layer.Values)
	if err != nil {
		return err
	}
	projects, err := json.Marshal(layer.SystemProjects)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO codelocal_runtime_config(user_id,scope,device_id,workspace_id,config,system_projects,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id,scope,device_id,workspace_id) DO UPDATE SET config=EXCLUDED.config,system_projects=EXCLUDED.system_projects,updated_at=EXCLUDED.updated_at`, userID, scope, deviceID, workspaceID, values, projects, time.Now().UnixMilli())
	return err
}

func (s *Store) PutRuntimeSecret(ctx context.Context, userID string, scope RuntimeScope, deviceID, workspaceID, name, value string) error {
	scopeValue, deviceID, workspaceID, err := normalizeRuntimeScope(scope, deviceID, workspaceID)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if !ValidRuntimeEnvKey(name) || value == "" {
		return errors.New("invalid runtime secret")
	}
	nonce, ciphertext, err := encryptRuntimeSecret(value)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO codelocal_runtime_secrets(user_id,scope,device_id,workspace_id,name,nonce,ciphertext,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(user_id,scope,device_id,workspace_id,name) DO UPDATE SET nonce=EXCLUDED.nonce,ciphertext=EXCLUDED.ciphertext,updated_at=EXCLUDED.updated_at`, userID, scopeValue, deviceID, workspaceID, name, nonce, ciphertext, time.Now().UnixMilli())
	return err
}

func (s *Store) DeleteRuntimeSecret(ctx context.Context, userID string, scope RuntimeScope, deviceID, workspaceID, name string) error {
	scopeValue, deviceID, workspaceID, err := normalizeRuntimeScope(scope, deviceID, workspaceID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM codelocal_runtime_secrets WHERE user_id=$1 AND scope=$2 AND device_id=$3 AND workspace_id=$4 AND name=$5`, userID, scopeValue, deviceID, workspaceID, strings.TrimSpace(name))
	return err
}

func (s *Store) ResolveRuntimeTarget(ctx context.Context, userID string, scope RuntimeScope, deviceID, workspaceID string) (RuntimeConfigSnapshot, error) {
	global, err := s.RuntimeSettingsLayer(ctx, userID, RuntimeScopeGlobal, "", "")
	if err != nil {
		return RuntimeConfigSnapshot{}, err
	}
	layers := []RuntimeConfigLayer{defaultRuntimeConfigLayer(), global}
	if scope == RuntimeScopeGlobal {
		return MergeRuntimeConfig(layers...), nil
	}
	device, err := s.RuntimeSettingsLayer(ctx, userID, RuntimeScopeDevice, deviceID, "")
	if err != nil {
		return RuntimeConfigSnapshot{}, err
	}
	layers = append(layers, device)
	if scope == RuntimeScopeDevice {
		return MergeRuntimeConfig(layers...), nil
	}
	if scope != RuntimeScopeWorkspace {
		return RuntimeConfigSnapshot{}, errors.New("invalid runtime config target")
	}
	workspace, err := s.RuntimeSettingsLayer(ctx, userID, RuntimeScopeWorkspace, deviceID, workspaceID)
	if err != nil {
		return RuntimeConfigSnapshot{}, err
	}
	return MergeRuntimeConfig(append(layers, workspace)...), nil
}

func (s *Store) ResolveRuntimeConfig(ctx context.Context, userID, deviceID, workspaceID string) (RuntimeConfigSnapshot, error) {
	return s.ResolveRuntimeTarget(ctx, userID, RuntimeScopeWorkspace, deviceID, workspaceID)
}

func (s *Store) materializeRuntimeScope(ctx context.Context, userID string, scope RuntimeScope, deviceID, workspaceID string, out map[string]string) error {
	scopeValue, deviceID, workspaceID, err := normalizeRuntimeScope(scope, deviceID, workspaceID)
	if err != nil {
		return err
	}
	rows, err := s.DB.Query(ctx, `SELECT name,nonce,ciphertext FROM codelocal_runtime_secrets WHERE user_id=$1 AND scope=$2 AND device_id=$3 AND workspace_id=$4 ORDER BY name`, userID, scopeValue, deviceID, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var nonce, ciphertext []byte
		if err := rows.Scan(&name, &nonce, &ciphertext); err != nil {
			return err
		}
		value, err := decryptRuntimeSecret(nonce, ciphertext)
		if err != nil {
			return err
		}
		out[name] = value
	}
	return rows.Err()
}

func (s *Store) MaterializeRuntimeSecrets(ctx context.Context, userID, deviceID, workspaceID string) (map[string]string, error) {
	out := map[string]string{}
	layers := []struct {
		scope                 RuntimeScope
		deviceID, workspaceID string
	}{
		{RuntimeScopeGlobal, "", ""},
		{RuntimeScopeDevice, deviceID, ""},
		{RuntimeScopeWorkspace, deviceID, workspaceID},
	}
	for _, layer := range layers {
		if err := s.materializeRuntimeScope(ctx, userID, layer.scope, layer.deviceID, layer.workspaceID, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}
