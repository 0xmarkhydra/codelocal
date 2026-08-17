package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type User struct {
	ID                string `json:"id"`
	Email             string `json:"email"`
	PasswordHash      string `json:"-"`
	PasswordSalt      string `json:"-"`
	PasswordChangedAt int64  `json:"passwordChangedAt,omitempty"`
	SecurityVersion   int64  `json:"-"`
	ReferralCode      string `json:"referralCode"`
	ReferredByCode    string `json:"referredByCode,omitempty"`
	CreatedAt         int64  `json:"createdAt"`
}

type SessionState struct {
	UserID    string          `json:"userId"`
	CSRF      string          `json:"csrf"`
	CreatedAt int64           `json:"createdAt"`
	Security  *SecuritySignal `json:"security,omitempty"`
	RiskUntil int64           `json:"riskUntil,omitempty"`
}

type Device struct {
	CredentialID         string `json:"credentialId"`
	PreviousCredentialID string `json:"-"`
	UserID               string `json:"userId"`
	DeviceID             string `json:"deviceId"`
	DeviceName           string `json:"deviceName"`
	PublicKey            string `json:"publicKey,omitempty"`
	SecretHash           string `json:"-"`
	CreatedAt            int64  `json:"createdAt"`
	LastSeenAt           int64  `json:"lastSeenAt"`
	RevokedAt            int64  `json:"revokedAt,omitempty"`
}

type Pairing struct {
	PairingID  string `json:"pairingId"`
	Code       string `json:"code"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	UserID     string `json:"userId,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	ApprovedAt int64  `json:"approvedAt,omitempty"`
	ClaimedAt  int64  `json:"claimedAt,omitempty"`
}

type Workspace struct {
	UserID            string         `json:"userId"`
	DeviceID          string         `json:"deviceId"`
	WorkspaceID       string         `json:"workspaceId"`
	WorkspaceName     string         `json:"workspaceName"`
	ProjectID         string         `json:"projectId,omitempty"`
	ProjectName       string         `json:"projectName,omitempty"`
	ProjectSource     string         `json:"projectSource,omitempty"`
	ProjectConfidence float64        `json:"projectConfidence,omitempty"`
	ProjectRoot       string         `json:"projectRoot,omitempty"`
	ProtocolVersion   int            `json:"protocolVersion,omitempty"`
	Capabilities      map[string]any `json:"capabilities"`
	CreatedAt         int64          `json:"createdAt"`
	LastSeenAt        int64          `json:"lastSeenAt"`
}

type OAuthClient struct {
	ClientID     string   `json:"clientId"`
	RedirectURIs []string `json:"redirectUris"`
	ClientName   string   `json:"clientName,omitempty"`
	CreatedAt    int64    `json:"createdAt"`
}

type OAuthCode struct {
	Code          string `json:"code"`
	UserID        string `json:"userId"`
	ClientID      string `json:"clientId"`
	RedirectURI   string `json:"redirectUri"`
	CodeChallenge string `json:"codeChallenge"`
	Resource      string `json:"resource"`
	Scope         string `json:"scope"`
	ExpiresAt     int64  `json:"expiresAt"`
}

type AuditEvent struct {
	UserID      string         `json:"userId,omitempty"`
	Event       string         `json:"event"`
	DeviceID    string         `json:"deviceId,omitempty"`
	WorkspaceID string         `json:"workspaceId,omitempty"`
	Detail      map[string]any `json:"detail,omitempty"`
	CreatedAt   int64          `json:"createdAt"`
}

type MCPUsageEvent struct {
	UserID          string `json:"userId"`
	SessionID       string `json:"sessionId,omitempty"`
	DeviceID        string `json:"deviceId,omitempty"`
	WorkspaceID     string `json:"workspaceId,omitempty"`
	Tool            string `json:"tool"`
	Calls           int64  `json:"calls"`
	InputBytes      int    `json:"inputBytes"`
	OutputBytes     int    `json:"outputBytes"`
	InputTokensEst  int    `json:"inputTokensEstimated"`
	OutputTokensEst int    `json:"outputTokensEstimated"`
	CreatedAt       int64  `json:"createdAt"`
}

type MCPUsageSummary struct {
	Calls           int64 `json:"calls"`
	InputBytes      int64 `json:"inputBytes"`
	OutputBytes     int64 `json:"outputBytes"`
	InputTokensEst  int64 `json:"inputTokensEstimated"`
	OutputTokensEst int64 `json:"outputTokensEstimated"`
	TotalTokensEst  int64 `json:"totalTokensEstimated"`
}

type AdminUser struct {
	ID               string `json:"id"`
	Email            string `json:"email"`
	ReferralCode     string `json:"referralCode"`
	ReferredByCode   string `json:"referredByCode,omitempty"`
	CreatedAt        int64  `json:"createdAt"`
	InviteCount      int64  `json:"inviteCount"`
	LastDeviceSeenAt int64  `json:"lastDeviceSeenAt,omitempty"`
	LastMCPUsedAt    int64  `json:"lastMcpUsedAt,omitempty"`
}

type Store struct {
	DB                         *pgxpool.Pool
	Redis                      *redis.Client
	ctx                        context.Context
	cancel                     context.CancelFunc
	wg                         sync.WaitGroup
	usageQ                     chan MCPUsageEvent
	usageConsumerID            string
	usageDropped               atomic.Uint64
	outboxWorkerID             string
	outboxWake                 chan struct{}
	canonicalEmbeddingProvider CanonicalEmbeddingProvider
	canonicalEmbeddingError    string
}

const (
	usageStreamKey       = "codelocal:mcp-usage:v1"
	usageStreamGroup     = "codelocal:mcp-usage-db:v1"
	usageConsumerLockKey = "codelocal:mcp-usage-db:leader"
)

func New(ctx context.Context) (*Store, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		return nil, errors.New("CodeLocal Cloud requires DATABASE_URL and REDIS_URL")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if raw := os.Getenv("CODELOCAL_DB_POOL_SIZE"); raw != "" {
		if value, parseErr := strconv.Atoi(raw); parseErr == nil && value > 0 {
			config.MaxConns = int32(value)
		}
	}
	config.MinConns = min32(2, config.MaxConns)
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 45 * time.Minute
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		db.Close()
		return nil, err
	}
	options.PoolSize = envInt("CODELOCAL_REDIS_POOL_SIZE", 32)
	rdb := redis.NewClient(options)
	if err := rdb.Ping(ctx).Err(); err != nil {
		db.Close()
		_ = rdb.Close()
		return nil, err
	}
	storeCtx, cancel := context.WithCancel(ctx)
	embeddingProvider, embeddingErr := canonicalEmbeddingProviderFromEnv()
	s := &Store{
		DB: db, Redis: rdb, ctx: storeCtx, cancel: cancel,
		usageQ: make(chan MCPUsageEvent, envInt("CODELOCAL_USAGE_LOCAL_QUEUE_SIZE", 8192)), usageConsumerID: RandomHex(12),
		outboxWorkerID: RandomHex(12), outboxWake: make(chan struct{}, 1),
		canonicalEmbeddingProvider: embeddingProvider,
	}
	if embeddingErr != nil {
		s.canonicalEmbeddingError = embeddingErr.Error()
		slog.Warn("canonical embedding provider unavailable", "error", embeddingErr)
	}
	if err := s.Migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.ensureCanonicalEmbeddingVectorSchema(ctx); err != nil {
		s.canonicalEmbeddingProvider = nil
		s.canonicalEmbeddingError = "canonical embedding vector schema unavailable: " + err.Error()
		slog.Warn("canonical embedding vector schema unavailable; derived semantic index disabled", "error", err)
	}
	if err := s.ensureUsageStreamGroup(ctx); err != nil {
		s.Close()
		return nil, err
	}
	s.wg.Add(5)
	go s.usageStreamProducer()
	go s.usageStreamConsumer()
	go s.retentionWorker()
	go s.durableOutboxWorker()
	go s.knowledgeHealthWorker()
	return s, nil
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func envInt(name string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func (s *Store) Close() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
	if s.Redis != nil {
		_ = s.Redis.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
}

type schemaMigration struct {
	version int
	sql     string
}

const schemaMigrationAdvisoryLockID int64 = 0x434F44454C4F4341 // "CODELOCA"; stable across replicas.

var nonTransactionalMigrationVersions = map[int]bool{14: true, 15: true, 16: true}

func knowledgeV2SchemaMigrations() []schemaMigration {
	return []schemaMigration{
		{26, durableOutboxMigrationSQL},
		{27, promotionCandidateMigrationSQL},
		{28, canonicalKnowledgeMigrationSQL},
		{29, promotionApprovalMigrationSQL},
		{30, knowledgeHealthMigrationSQL},
		{31, promotionSourcesMigrationSQL},
		{32, repositoryIdentityMigrationSQL},
		{33, knowledgeV2ReadinessMigrationSQL},
		{34, portableLearnedSkillsMigrationSQL},
		{35, collectiveIntelligenceMigrationSQL},
		{36, canonicalKnowledgeGraphMigrationSQL},
		{37, canonicalKnowledgeGraphStateMigrationSQL},
		{38, canonicalKnowledgeEmbeddingMigrationSQL},
		{39, canonicalEmbeddingShadowMigrationSQL},
		{40, canonicalSemanticCanaryMigrationSQL},
	}
}

func accountSchemaMigrations() []schemaMigration {
	return []schemaMigration{
		{41, `ALTER TABLE codelocal_users ADD COLUMN IF NOT EXISTS password_changed_at BIGINT NOT NULL DEFAULT 0;`},
		{42, `ALTER TABLE codelocal_users ADD COLUMN IF NOT EXISTS security_version BIGINT NOT NULL DEFAULT 1;`},
		{43, `ALTER TABLE codelocal_devices ADD COLUMN IF NOT EXISTS public_key TEXT;`},
	}
}

func validateSchemaMigrationPlan(migrations []schemaMigration) error {
	if len(migrations) == 0 {
		return errors.New("schema migration plan is empty")
	}
	for index, migration := range migrations {
		expected := index + 1
		if migration.version != expected {
			return fmt.Errorf("schema migration plan must be contiguous: index=%d version=%d want=%d", index, migration.version, expected)
		}
		if strings.TrimSpace(migration.sql) == "" {
			return fmt.Errorf("schema migration %d has empty SQL", migration.version)
		}
	}
	return nil
}

type SchemaMigrationStatus struct {
	CurrentVersion       int    `json:"currentVersion"`
	TargetVersion        int    `json:"targetVersion"`
	AppliedCount         int    `json:"appliedCount"`
	UpToDate             bool   `json:"upToDate"`
	ProjectBrainPlanHash string `json:"projectBrainPlanHash"`
}

func LatestSchemaMigrationVersion() int {
	if migrations := accountSchemaMigrations(); len(migrations) > 0 {
		return migrations[len(migrations)-1].version
	}
	migrations := knowledgeV2SchemaMigrations()
	if len(migrations) == 0 {
		return 25
	}
	return migrations[len(migrations)-1].version
}

func ProjectBrainMigrationPlanHash() string {
	hash := sha256.New()
	for _, migration := range knowledgeV2SchemaMigrations() {
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00", migration.version, strings.TrimSpace(migration.sql))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func schemaMigrationStatus(currentVersion, appliedCount int) SchemaMigrationStatus {
	target := LatestSchemaMigrationVersion()
	return SchemaMigrationStatus{
		CurrentVersion:       currentVersion,
		TargetVersion:        target,
		AppliedCount:         appliedCount,
		UpToDate:             currentVersion == target && appliedCount == target,
		ProjectBrainPlanHash: ProjectBrainMigrationPlanHash(),
	}
}

func validateDatabaseSchemaHistory(currentVersion, appliedCount, targetVersion int) error {
	if currentVersion < 0 || appliedCount < 0 || targetVersion < 1 {
		return errors.New("invalid database schema migration state")
	}
	if currentVersion > targetVersion {
		return fmt.Errorf("database schema version %d is newer than binary target %d", currentVersion, targetVersion)
	}
	if currentVersion != appliedCount {
		return fmt.Errorf("database schema migration history is non-contiguous: maxVersion=%d appliedCount=%d", currentVersion, appliedCount)
	}
	return nil
}

func (s *Store) SchemaMigrationStatus(ctx context.Context) (SchemaMigrationStatus, error) {
	if s == nil || s.DB == nil {
		return schemaMigrationStatus(0, 0), errors.New("schema migration store unavailable")
	}
	var currentVersion, appliedCount int
	if err := s.DB.QueryRow(ctx, `SELECT COALESCE(MAX(version),0),COUNT(*) FROM codelocal_schema_migrations`).Scan(&currentVersion, &appliedCount); err != nil {
		return schemaMigrationStatus(0, 0), err
	}
	return schemaMigrationStatus(currentVersion, appliedCount), nil
}

func (s *Store) Migrate(ctx context.Context) error {
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	locked := false
	defer func() {
		if locked {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var unlocked bool
			if unlockErr := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock($1)`, schemaMigrationAdvisoryLockID).Scan(&unlocked); unlockErr != nil {
				slog.Warn("schema migration advisory unlock failed; closing dedicated session", "error", unlockErr)
				raw := conn.Hijack()
				_ = raw.Close(unlockCtx)
				return
			}
		}
		conn.Release()
	}()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, schemaMigrationAdvisoryLockID); err != nil {
		return err
	}
	locked = true
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS codelocal_schema_migrations (version INTEGER PRIMARY KEY, applied_at BIGINT NOT NULL)`); err != nil {
		return err
	}
	migrations := []schemaMigration{
		{1, `
CREATE TABLE IF NOT EXISTS codelocal_users (
 id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, password_salt TEXT NOT NULL, created_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS codelocal_devices (
 credential_id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 device_id TEXT NOT NULL, device_name TEXT NOT NULL, secret_hash TEXT NOT NULL, created_at BIGINT NOT NULL, last_seen_at BIGINT NOT NULL, revoked_at BIGINT,
 UNIQUE(user_id,device_id)
);
CREATE TABLE IF NOT EXISTS codelocal_pairings (
 pairing_id TEXT PRIMARY KEY, code TEXT NOT NULL, device_id TEXT NOT NULL, device_name TEXT NOT NULL,
 user_id TEXT REFERENCES codelocal_users(id) ON DELETE CASCADE, created_at BIGINT NOT NULL, expires_at BIGINT NOT NULL, approved_at BIGINT, claimed_at BIGINT
);
CREATE TABLE IF NOT EXISTS codelocal_workspaces (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE, device_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 workspace_name TEXT NOT NULL, project_root TEXT, protocol_version INTEGER, capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
 created_at BIGINT NOT NULL, last_seen_at BIGINT NOT NULL, PRIMARY KEY(user_id,device_id,workspace_id)
);
CREATE TABLE IF NOT EXISTS codelocal_oauth_clients (
 client_id TEXT PRIMARY KEY, redirect_uris JSONB NOT NULL, client_name TEXT, created_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS codelocal_oauth_codes (
 code TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 client_id TEXT NOT NULL REFERENCES codelocal_oauth_clients(client_id) ON DELETE CASCADE,
 redirect_uri TEXT NOT NULL, code_challenge TEXT NOT NULL, resource TEXT NOT NULL, scope TEXT NOT NULL, expires_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS codelocal_permissions (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE, workspace_id TEXT,
 capability TEXT NOT NULL, decision TEXT NOT NULL CHECK (decision IN ('allow','ask','deny')), updated_at BIGINT NOT NULL,
 UNIQUE(user_id,workspace_id,capability)
);
CREATE TABLE IF NOT EXISTS codelocal_audit_logs (
 id TEXT PRIMARY KEY, user_id TEXT REFERENCES codelocal_users(id) ON DELETE SET NULL, event TEXT NOT NULL,
 device_id TEXT, workspace_id TEXT, detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_devices_user ON codelocal_devices(user_id);
CREATE INDEX IF NOT EXISTS idx_codelocal_workspaces_user ON codelocal_workspaces(user_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_audit_user ON codelocal_audit_logs(user_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_pairings_expires ON codelocal_pairings(expires_at);
`},
		{2, `UPDATE codelocal_workspaces SET project_root=NULL WHERE project_root IS NOT NULL;`},
		{3, `
CREATE TABLE IF NOT EXISTS codelocal_mcp_usage (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 session_id TEXT, device_id TEXT, workspace_id TEXT, tool TEXT NOT NULL,
 input_bytes BIGINT NOT NULL DEFAULT 0, output_bytes BIGINT NOT NULL DEFAULT 0,
 input_tokens_est BIGINT NOT NULL DEFAULT 0, output_tokens_est BIGINT NOT NULL DEFAULT 0,
 created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_mcp_usage_user_time ON codelocal_mcp_usage(user_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_mcp_usage_workspace_time ON codelocal_mcp_usage(user_id,workspace_id,created_at DESC);
`},
		{4, `
CREATE TABLE IF NOT EXISTS codelocal_mcp_usage_rollup (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 bucket_start BIGINT NOT NULL,
 device_id TEXT NOT NULL DEFAULT '', workspace_id TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL,
 calls BIGINT NOT NULL DEFAULT 0,
 input_bytes BIGINT NOT NULL DEFAULT 0, output_bytes BIGINT NOT NULL DEFAULT 0,
 input_tokens_est BIGINT NOT NULL DEFAULT 0, output_tokens_est BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(user_id,bucket_start,device_id,workspace_id,tool)
);
INSERT INTO codelocal_mcp_usage_rollup(user_id,bucket_start,device_id,workspace_id,tool,calls,input_bytes,output_bytes,input_tokens_est,output_tokens_est)
SELECT user_id,(created_at/3600000)*3600000,COALESCE(device_id,''),COALESCE(workspace_id,''),tool,COUNT(*),SUM(input_bytes),SUM(output_bytes),SUM(input_tokens_est),SUM(output_tokens_est)
FROM codelocal_mcp_usage
GROUP BY user_id,(created_at/3600000)*3600000,COALESCE(device_id,''),COALESCE(workspace_id,''),tool
ON CONFLICT(user_id,bucket_start,device_id,workspace_id,tool) DO UPDATE SET
 calls=codelocal_mcp_usage_rollup.calls+EXCLUDED.calls,
 input_bytes=codelocal_mcp_usage_rollup.input_bytes+EXCLUDED.input_bytes,
 output_bytes=codelocal_mcp_usage_rollup.output_bytes+EXCLUDED.output_bytes,
 input_tokens_est=codelocal_mcp_usage_rollup.input_tokens_est+EXCLUDED.input_tokens_est,
 output_tokens_est=codelocal_mcp_usage_rollup.output_tokens_est+EXCLUDED.output_tokens_est;
DROP TABLE codelocal_mcp_usage;
ALTER TABLE codelocal_mcp_usage_rollup RENAME TO codelocal_mcp_usage;
CREATE INDEX IF NOT EXISTS idx_codelocal_mcp_usage_user_time ON codelocal_mcp_usage(user_id,bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_mcp_usage_workspace_time ON codelocal_mcp_usage(user_id,workspace_id,bucket_start DESC);
`},
		{5, `DELETE FROM codelocal_audit_logs WHERE event <> 'terminal.executed';`},
		{6, `
ALTER TABLE codelocal_users ADD COLUMN IF NOT EXISTS referral_code TEXT;
ALTER TABLE codelocal_users ADD COLUMN IF NOT EXISTS referred_by_code TEXT;
UPDATE codelocal_users
SET referral_code='U' || UPPER(SUBSTR(MD5(id),1,16))
WHERE referral_code IS NULL OR BTRIM(referral_code)='';
UPDATE codelocal_users
SET referred_by_code='MMON'
WHERE referred_by_code IS NULL OR BTRIM(referred_by_code)='';
ALTER TABLE codelocal_users ALTER COLUMN referral_code SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_users_referral_code_ci ON codelocal_users ((UPPER(referral_code)));
CREATE INDEX IF NOT EXISTS idx_codelocal_users_referred_by_code_ci ON codelocal_users ((UPPER(referred_by_code)));
`},
		{7, `
UPDATE codelocal_users
SET referral_code='U' || UPPER(SUBSTR(MD5(id || ':reserved-mmon'),1,16)),
    referred_by_code=COALESCE(NULLIF(BTRIM(referred_by_code),''),'MMON')
WHERE UPPER(referral_code)='MMON';
`},
		{8, `
CREATE TABLE IF NOT EXISTS codelocal_mcp_usage_total (
 user_id TEXT PRIMARY KEY REFERENCES codelocal_users(id) ON DELETE CASCADE,
 calls BIGINT NOT NULL DEFAULT 0,
 input_bytes BIGINT NOT NULL DEFAULT 0, output_bytes BIGINT NOT NULL DEFAULT 0,
 input_tokens_est BIGINT NOT NULL DEFAULT 0, output_tokens_est BIGINT NOT NULL DEFAULT 0,
 last_used_at BIGINT NOT NULL DEFAULT 0
);
INSERT INTO codelocal_mcp_usage_total(user_id,calls,input_bytes,output_bytes,input_tokens_est,output_tokens_est,last_used_at)
SELECT user_id,SUM(calls),SUM(input_bytes),SUM(output_bytes),SUM(input_tokens_est),SUM(output_tokens_est),MAX(bucket_start)
FROM codelocal_mcp_usage
GROUP BY user_id
ON CONFLICT(user_id) DO UPDATE SET
 calls=codelocal_mcp_usage_total.calls+EXCLUDED.calls,
 input_bytes=codelocal_mcp_usage_total.input_bytes+EXCLUDED.input_bytes,
 output_bytes=codelocal_mcp_usage_total.output_bytes+EXCLUDED.output_bytes,
 input_tokens_est=codelocal_mcp_usage_total.input_tokens_est+EXCLUDED.input_tokens_est,
 output_tokens_est=codelocal_mcp_usage_total.output_tokens_est+EXCLUDED.output_tokens_est,
 last_used_at=GREATEST(codelocal_mcp_usage_total.last_used_at,EXCLUDED.last_used_at);
DROP TABLE codelocal_mcp_usage;
ALTER TABLE codelocal_mcp_usage_total RENAME TO codelocal_mcp_usage;
`},
		{9, `
DO $$
DECLARE
 rec RECORD;
 attempt INTEGER;
 candidate TEXT;
BEGIN
 CREATE TEMP TABLE codelocal_referral_short_map (
  user_id TEXT PRIMARY KEY,
  old_code TEXT NOT NULL,
  temp_code TEXT NOT NULL UNIQUE,
  new_code TEXT NOT NULL UNIQUE
 ) ON COMMIT DROP;

 FOR rec IN SELECT id, referral_code FROM codelocal_users ORDER BY id LOOP
  attempt := 0;
  LOOP
   candidate := UPPER(SUBSTR(MD5(rec.id || ':referral-v2:' || attempt::TEXT),1,6));
   BEGIN
    INSERT INTO codelocal_referral_short_map(user_id,old_code,temp_code,new_code)
    VALUES(rec.id,rec.referral_code,'__REF6__' || rec.id,candidate);
    EXIT;
   EXCEPTION WHEN unique_violation THEN
    attempt := attempt + 1;
   END;
  END LOOP;
 END LOOP;

 UPDATE codelocal_users child
 SET referred_by_code=map.new_code
 FROM codelocal_referral_short_map map
 WHERE UPPER(COALESCE(child.referred_by_code,''))=UPPER(map.old_code);

 UPDATE codelocal_users user_row
 SET referral_code=map.temp_code
 FROM codelocal_referral_short_map map
 WHERE user_row.id=map.user_id;

 UPDATE codelocal_users user_row
 SET referral_code=map.new_code
 FROM codelocal_referral_short_map map
 WHERE user_row.id=map.user_id;
END $$;
`},
		{10, `
CREATE TABLE IF NOT EXISTS codelocal_mcp_usage_batches (
 id TEXT PRIMARY KEY,
 processed_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_mcp_usage_batches_processed ON codelocal_mcp_usage_batches(processed_at);
`},
		{11, `
CREATE TABLE IF NOT EXISTS codelocal_memories (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 workspace_id TEXT NOT NULL,
 task_id TEXT,
 level TEXT NOT NULL CHECK (level IN ('event','scenario','workspace')),
 summary TEXT NOT NULL,
 branch TEXT,
 files JSONB NOT NULL DEFAULT '[]'::jsonb,
 symbols JSONB NOT NULL DEFAULT '[]'::jsonb,
 confidence DOUBLE PRECISION NOT NULL DEFAULT 0.7,
 importance DOUBLE PRECISION NOT NULL DEFAULT 0.5,
 idempotency_key TEXT,
 embedding_model TEXT,
 embedding_dimension INTEGER,
 created_at BIGINT NOT NULL,
 last_used_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_memories_idempotency ON codelocal_memories(user_id,workspace_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codelocal_memories_scope ON codelocal_memories(user_id,workspace_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_memories_task ON codelocal_memories(user_id,workspace_id,task_id);
CREATE INDEX IF NOT EXISTS idx_codelocal_memories_fts ON codelocal_memories USING GIN (to_tsvector('simple',summary));
`},
		{12, `
ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'workspace';
ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS kind TEXT;
ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'task';
ALTER TABLE codelocal_memories ALTER COLUMN workspace_id DROP NOT NULL;
UPDATE codelocal_memories SET scope='workspace' WHERE scope IS NULL OR scope='';
ALTER TABLE codelocal_memories DROP CONSTRAINT IF EXISTS codelocal_memories_scope_check;
ALTER TABLE codelocal_memories ADD CONSTRAINT codelocal_memories_scope_check
 CHECK ((scope='global' AND workspace_id IS NULL) OR (scope='workspace' AND workspace_id IS NOT NULL)) NOT VALID;
ALTER TABLE codelocal_memories VALIDATE CONSTRAINT codelocal_memories_scope_check;

CREATE TABLE IF NOT EXISTS codelocal_memory_nodes (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 workspace_id TEXT,
 scope TEXT NOT NULL CHECK (scope IN ('global','workspace')),
 kind TEXT NOT NULL,
 canonical_name TEXT NOT NULL,
 summary TEXT NOT NULL DEFAULT '',
 confidence DOUBLE PRECISION NOT NULL DEFAULT 0.7,
 importance DOUBLE PRECISION NOT NULL DEFAULT 0.5,
 valid_from BIGINT NOT NULL,
 valid_to BIGINT,
 first_seen_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 source_type TEXT NOT NULL DEFAULT 'memory',
 source_session_id TEXT,
 source_memory_id TEXT REFERENCES codelocal_memories(id) ON DELETE SET NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 CHECK ((scope='global' AND workspace_id IS NULL) OR (scope='workspace' AND workspace_id IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_memory_nodes_identity
 ON codelocal_memory_nodes(user_id,scope,(COALESCE(workspace_id,'')),kind,canonical_name);
CREATE INDEX IF NOT EXISTS idx_codelocal_memory_nodes_scope
 ON codelocal_memory_nodes(user_id,scope,workspace_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_memory_nodes_source_memory
 ON codelocal_memory_nodes(user_id,source_memory_id) WHERE source_memory_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS codelocal_memory_edges (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 workspace_id TEXT,
 scope TEXT NOT NULL CHECK (scope IN ('global','workspace')),
 from_node_id TEXT NOT NULL REFERENCES codelocal_memory_nodes(id) ON DELETE CASCADE,
 to_node_id TEXT NOT NULL REFERENCES codelocal_memory_nodes(id) ON DELETE CASCADE,
 relation TEXT NOT NULL,
 confidence DOUBLE PRECISION NOT NULL DEFAULT 0.7,
 importance DOUBLE PRECISION NOT NULL DEFAULT 0.5,
 valid_from BIGINT NOT NULL,
 valid_to BIGINT,
 first_seen_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 source_session_id TEXT,
 source_memory_id TEXT REFERENCES codelocal_memories(id) ON DELETE SET NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 CHECK ((scope='global' AND workspace_id IS NULL) OR (scope='workspace' AND workspace_id IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_memory_edges_identity
 ON codelocal_memory_edges(user_id,scope,(COALESCE(workspace_id,'')),from_node_id,to_node_id,relation);
CREATE INDEX IF NOT EXISTS idx_codelocal_memory_edges_from
 ON codelocal_memory_edges(user_id,from_node_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_memory_edges_to
 ON codelocal_memory_edges(user_id,to_node_id,last_seen_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_memory_node_aliases (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 workspace_id TEXT,
 scope TEXT NOT NULL CHECK (scope IN ('global','workspace')),
 node_id TEXT NOT NULL REFERENCES codelocal_memory_nodes(id) ON DELETE CASCADE,
 alias TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 CHECK ((scope='global' AND workspace_id IS NULL) OR (scope='workspace' AND workspace_id IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_memory_alias_identity
 ON codelocal_memory_node_aliases(user_id,scope,(COALESCE(workspace_id,'')),alias);

CREATE TABLE IF NOT EXISTS codelocal_memory_sources (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 workspace_id TEXT,
 scope TEXT NOT NULL CHECK (scope IN ('global','workspace')),
 node_id TEXT NOT NULL REFERENCES codelocal_memory_nodes(id) ON DELETE CASCADE,
 source_type TEXT NOT NULL,
 source_session_id TEXT,
 source_memory_id TEXT REFERENCES codelocal_memories(id) ON DELETE CASCADE,
 created_at BIGINT NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 CHECK ((scope='global' AND workspace_id IS NULL) OR (scope='workspace' AND workspace_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_codelocal_memory_sources_scope
 ON codelocal_memory_sources(user_id,scope,workspace_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_memory_sources_node
 ON codelocal_memory_sources(user_id,node_id,created_at DESC);
`},
		{13, `
ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT 0;
UPDATE codelocal_memories SET updated_at=0 WHERE updated_at IS NULL;
ALTER TABLE codelocal_memories ALTER COLUMN updated_at SET DEFAULT 0;
ALTER TABLE codelocal_memories ALTER COLUMN updated_at SET NOT NULL;
`},
		{14, `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_codelocal_memories_idempotency_scope
 ON codelocal_memories(user_id,scope,(COALESCE(workspace_id,'')),idempotency_key)
 WHERE idempotency_key IS NOT NULL;`},
		{15, `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_codelocal_memories_global
 ON codelocal_memories(user_id,updated_at DESC) WHERE scope='global';`},
		{16, `DROP INDEX CONCURRENTLY IF EXISTS idx_codelocal_memories_idempotency;`},
		{17, `
CREATE TABLE IF NOT EXISTS codelocal_projects (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 project_id TEXT NOT NULL,
 name TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_projects_user_seen ON codelocal_projects(user_id,last_seen_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_repositories (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 repository_id TEXT NOT NULL,
 remote TEXT,
 lineage TEXT,
 identity_source TEXT NOT NULL CHECK (identity_source IN ('remote','lineage')),
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,repository_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_repositories_user_seen ON codelocal_repositories(user_id,last_seen_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_project_repositories (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 repository_id TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id,repository_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,repository_id) REFERENCES codelocal_repositories(user_id,repository_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_repositories_repo ON codelocal_project_repositories(user_id,repository_id,project_id);

CREATE TABLE IF NOT EXISTS codelocal_workspace_projects (
 user_id TEXT NOT NULL,
 device_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 source TEXT NOT NULL,
 confidence DOUBLE PRECISION NOT NULL DEFAULT 1,
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,device_id,workspace_id),
 FOREIGN KEY(user_id,device_id,workspace_id) REFERENCES codelocal_workspaces(user_id,device_id,workspace_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_workspace_projects_project ON codelocal_workspace_projects(user_id,project_id,last_seen_at DESC);
`},
		{18, `
CREATE TABLE IF NOT EXISTS codelocal_learned_skill_metadata (
 user_id TEXT NOT NULL,
 device_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL,
 skill_id TEXT NOT NULL,
 intent TEXT NOT NULL,
 task_kind TEXT,
 status TEXT NOT NULL,
 confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
 success_count INTEGER NOT NULL DEFAULT 0,
 failure_count INTEGER NOT NULL DEFAULT 0,
 step_count INTEGER NOT NULL DEFAULT 0,
 updated_at BIGINT NOT NULL,
 last_used_at BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(user_id,device_id,workspace_id,skill_id),
 FOREIGN KEY(user_id,device_id,workspace_id) REFERENCES codelocal_workspaces(user_id,device_id,workspace_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_learned_skill_metadata_user ON codelocal_learned_skill_metadata(user_id,last_used_at DESC,updated_at DESC);
`},
		{19, `
CREATE TABLE IF NOT EXISTS codelocal_workspace_repositories (
 user_id TEXT NOT NULL,
 device_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL,
 repository_id TEXT NOT NULL,
 relative_path TEXT NOT NULL DEFAULT '.',
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,device_id,workspace_id,repository_id),
 FOREIGN KEY(user_id,device_id,workspace_id) REFERENCES codelocal_workspaces(user_id,device_id,workspace_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,repository_id) REFERENCES codelocal_repositories(user_id,repository_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_workspace_repositories_repo
 ON codelocal_workspace_repositories(user_id,repository_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_workspace_repositories_path
 ON codelocal_workspace_repositories(user_id,workspace_id,relative_path);

ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS project_id TEXT;
ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS repository_id TEXT;
ALTER TABLE codelocal_memories DROP CONSTRAINT IF EXISTS codelocal_memories_project_fk;
ALTER TABLE codelocal_memories ADD CONSTRAINT codelocal_memories_project_fk
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE NOT VALID;
ALTER TABLE codelocal_memories VALIDATE CONSTRAINT codelocal_memories_project_fk;
ALTER TABLE codelocal_memories DROP CONSTRAINT IF EXISTS codelocal_memories_repository_fk;
ALTER TABLE codelocal_memories ADD CONSTRAINT codelocal_memories_repository_fk
 FOREIGN KEY(user_id,repository_id) REFERENCES codelocal_repositories(user_id,repository_id) ON DELETE CASCADE NOT VALID;
ALTER TABLE codelocal_memories VALIDATE CONSTRAINT codelocal_memories_repository_fk;
ALTER TABLE codelocal_memories DROP CONSTRAINT IF EXISTS codelocal_memories_scope_check;
ALTER TABLE codelocal_memories ADD CONSTRAINT codelocal_memories_scope_check CHECK (
 (scope='global' AND workspace_id IS NULL AND project_id IS NULL AND repository_id IS NULL) OR
 (scope='project' AND workspace_id IS NULL AND project_id IS NOT NULL AND repository_id IS NULL) OR
 (scope='repository' AND workspace_id IS NULL AND project_id IS NOT NULL AND repository_id IS NOT NULL) OR
 (scope='workspace' AND workspace_id IS NOT NULL AND project_id IS NULL AND repository_id IS NULL)
) NOT VALID;
ALTER TABLE codelocal_memories VALIDATE CONSTRAINT codelocal_memories_scope_check;

DROP INDEX IF EXISTS idx_codelocal_memories_idempotency_scope;
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_memories_idempotency_scope
 ON codelocal_memories(user_id,scope,(COALESCE(workspace_id,'')),(COALESCE(project_id,'')),(COALESCE(repository_id,'')),idempotency_key)
 WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codelocal_memories_project
 ON codelocal_memories(user_id,project_id,updated_at DESC) WHERE scope='project';
CREATE INDEX IF NOT EXISTS idx_codelocal_memories_repository
 ON codelocal_memories(user_id,project_id,repository_id,updated_at DESC) WHERE scope='repository';
`},
		{20, `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_sources (
 user_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 repository_id TEXT,
 provider TEXT NOT NULL,
 source_type TEXT NOT NULL,
 canonical_path TEXT NOT NULL,
 classification TEXT NOT NULL CHECK (classification IN ('public_project','team_project','private_project','local_private','sensitive')),
 status TEXT NOT NULL CHECK (status IN ('active','stale','conflicted','superseded','revoked')),
 active_revision_id TEXT,
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 valid_from BIGINT NOT NULL,
 valid_to BIGINT,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(user_id,source_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,project_id,repository_id) REFERENCES codelocal_project_repositories(user_id,project_id,repository_id) ON DELETE CASCADE,
 CHECK (BTRIM(provider) <> ''),
 CHECK (BTRIM(source_type) <> ''),
 CHECK (BTRIM(canonical_path) <> ''),
 CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_knowledge_sources_identity
 ON codelocal_knowledge_sources(user_id,project_id,(COALESCE(repository_id,'')),provider,source_type,canonical_path);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_sources_project
 ON codelocal_knowledge_sources(user_id,project_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_sources_repository
 ON codelocal_knowledge_sources(user_id,project_id,repository_id,last_seen_at DESC) WHERE repository_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_sources_status
 ON codelocal_knowledge_sources(user_id,status,last_seen_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_knowledge_source_revisions (
 user_id TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 content_hash TEXT,
 semantic_hash TEXT,
 parser_fingerprint TEXT NOT NULL,
 adapter_version TEXT NOT NULL,
 parser_version TEXT NOT NULL,
 semantic_normalizer_version TEXT NOT NULL,
 git_blob_oid TEXT,
 tombstone BOOLEAN NOT NULL DEFAULT FALSE,
 created_at BIGINT NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(user_id,revision_id),
 FOREIGN KEY(user_id,source_id) REFERENCES codelocal_knowledge_sources(user_id,source_id) ON DELETE CASCADE,
 CHECK (BTRIM(parser_fingerprint) <> ''),
 CHECK (BTRIM(adapter_version) <> ''),
 CHECK (BTRIM(parser_version) <> ''),
 CHECK (BTRIM(semantic_normalizer_version) <> ''),
 CHECK ((tombstone=TRUE AND content_hash IS NULL AND semantic_hash IS NULL) OR (tombstone=FALSE AND content_hash IS NOT NULL AND BTRIM(content_hash) <> ''))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_knowledge_revisions_fingerprint
 ON codelocal_knowledge_source_revisions(user_id,source_id,(COALESCE(content_hash,'')),(COALESCE(semantic_hash,'')),parser_fingerprint,adapter_version,parser_version,semantic_normalizer_version,tombstone);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_revisions_source
 ON codelocal_knowledge_source_revisions(user_id,source_id,created_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_knowledge_source_observations (
 user_id TEXT NOT NULL,
 observation_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 base_revision_id TEXT,
 device_id TEXT,
 workspace_id TEXT,
 branch TEXT,
 git_commit TEXT,
 first_seen_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(user_id,observation_id),
 FOREIGN KEY(user_id,source_id) REFERENCES codelocal_knowledge_sources(user_id,source_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,revision_id) REFERENCES codelocal_knowledge_source_revisions(user_id,revision_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,base_revision_id) REFERENCES codelocal_knowledge_source_revisions(user_id,revision_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_observations_source
 ON codelocal_knowledge_source_observations(user_id,source_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_observations_revision
 ON codelocal_knowledge_source_observations(user_id,revision_id,last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_observations_branch
 ON codelocal_knowledge_source_observations(user_id,source_id,branch,last_seen_at DESC) WHERE branch IS NOT NULL;

CREATE OR REPLACE FUNCTION codelocal_reject_knowledge_revision_update() RETURNS trigger AS $$
BEGIN
 RAISE EXCEPTION 'knowledge source revisions are immutable';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_codelocal_knowledge_revision_immutable ON codelocal_knowledge_source_revisions;
CREATE TRIGGER trg_codelocal_knowledge_revision_immutable
 BEFORE UPDATE ON codelocal_knowledge_source_revisions
 FOR EACH ROW EXECUTE FUNCTION codelocal_reject_knowledge_revision_update();
`},
		{21, `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_conflicts (
 user_id TEXT NOT NULL,
 conflict_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 active_revision_id TEXT NOT NULL,
 candidate_revision_id TEXT NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('open','resolved','dismissed')),
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 resolved_at BIGINT,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(user_id,conflict_id),
 FOREIGN KEY(user_id,source_id) REFERENCES codelocal_knowledge_sources(user_id,source_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,active_revision_id) REFERENCES codelocal_knowledge_source_revisions(user_id,revision_id),
 FOREIGN KEY(user_id,candidate_revision_id) REFERENCES codelocal_knowledge_source_revisions(user_id,revision_id),
 CHECK (active_revision_id <> candidate_revision_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_conflicts_source
 ON codelocal_knowledge_conflicts(user_id,source_id,updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_conflicts_open
 ON codelocal_knowledge_conflicts(user_id,updated_at DESC) WHERE status='open';
`},
		{22, `
CREATE TABLE IF NOT EXISTS codelocal_experiences (
 user_id TEXT NOT NULL,
 experience_id TEXT NOT NULL,
 project_id TEXT,
 repository_id TEXT,
 workspace_id TEXT,
 device_id TEXT,
 task_id TEXT,
 task_kind TEXT,
 objective TEXT NOT NULL,
 branch TEXT,
 files JSONB NOT NULL DEFAULT '[]'::jsonb,
 symbols JSONB NOT NULL DEFAULT '[]'::jsonb,
 checks JSONB NOT NULL DEFAULT '[]'::jsonb,
 outcome TEXT NOT NULL CHECK (outcome IN ('succeeded','failed')),
 root_cause TEXT,
 skill_id TEXT,
 rules_hash TEXT,
 context_hash TEXT,
 verification_summary TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(user_id,experience_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_experiences_project
 ON codelocal_experiences(user_id,project_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_experiences_repository
 ON codelocal_experiences(user_id,project_id,repository_id,created_at DESC) WHERE repository_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codelocal_experiences_task
 ON codelocal_experiences(user_id,task_kind,created_at DESC);
`},
		{23, `
ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS lifecycle_status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE codelocal_memories DROP CONSTRAINT IF EXISTS codelocal_memories_lifecycle_status_check;
ALTER TABLE codelocal_memories ADD CONSTRAINT codelocal_memories_lifecycle_status_check
 CHECK (lifecycle_status IN ('observed','confirmed','active','stale','superseded','invalidated'));
CREATE INDEX IF NOT EXISTS idx_codelocal_memories_lifecycle
 ON codelocal_memories(user_id,lifecycle_status,updated_at DESC);
`},
		{24, `
CREATE TABLE IF NOT EXISTS codelocal_organizations (
 owner_user_id TEXT NOT NULL,
 organization_id TEXT NOT NULL,
 name TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(owner_user_id,organization_id),
 FOREIGN KEY(owner_user_id) REFERENCES codelocal_users(id) ON DELETE CASCADE,
 CHECK (BTRIM(name) <> '')
);
CREATE TABLE IF NOT EXISTS codelocal_organization_members (
 owner_user_id TEXT NOT NULL,
 organization_id TEXT NOT NULL,
 member_user_id TEXT NOT NULL,
 role TEXT NOT NULL CHECK (role IN ('owner','admin','member','viewer')),
 status TEXT NOT NULL CHECK (status IN ('active','invited','revoked')),
 added_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(owner_user_id,organization_id,member_user_id),
 FOREIGN KEY(owner_user_id,organization_id) REFERENCES codelocal_organizations(owner_user_id,organization_id) ON DELETE CASCADE,
 FOREIGN KEY(member_user_id) REFERENCES codelocal_users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_org_members_member
 ON codelocal_organization_members(member_user_id,status,updated_at DESC);
CREATE TABLE IF NOT EXISTS codelocal_project_organizations (
 owner_user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 organization_id TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(owner_user_id,project_id,organization_id),
 FOREIGN KEY(owner_user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(owner_user_id,organization_id) REFERENCES codelocal_organizations(owner_user_id,organization_id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS codelocal_organization_rules (
 owner_user_id TEXT NOT NULL,
 organization_id TEXT NOT NULL,
 rule_id TEXT NOT NULL,
 rule_text TEXT NOT NULL,
 apply_to JSONB NOT NULL DEFAULT '[]'::jsonb,
 required BOOLEAN NOT NULL DEFAULT TRUE,
 status TEXT NOT NULL CHECK (status IN ('active','revoked')),
 revision BIGINT NOT NULL DEFAULT 1,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(owner_user_id,organization_id,rule_id),
 FOREIGN KEY(owner_user_id,organization_id) REFERENCES codelocal_organizations(owner_user_id,organization_id) ON DELETE CASCADE,
 CHECK (BTRIM(rule_text) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_org_rules_active
 ON codelocal_organization_rules(owner_user_id,organization_id,updated_at DESC) WHERE status='active';
`},
		{25, `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_branch_heads (
 user_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 branch TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,source_id,branch),
 FOREIGN KEY(user_id,source_id) REFERENCES codelocal_knowledge_sources(user_id,source_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,revision_id) REFERENCES codelocal_knowledge_source_revisions(user_id,revision_id),
 CHECK (BTRIM(branch) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_branch_heads_revision
 ON codelocal_knowledge_branch_heads(user_id,revision_id,updated_at DESC);
INSERT INTO codelocal_knowledge_branch_heads(user_id,source_id,branch,revision_id,updated_at)
SELECT DISTINCT ON (obs.user_id,obs.source_id,obs.branch)
 obs.user_id,obs.source_id,obs.branch,obs.revision_id,obs.last_seen_at
FROM codelocal_knowledge_source_observations obs
JOIN codelocal_knowledge_sources src
 ON src.user_id=obs.user_id AND src.source_id=obs.source_id
WHERE obs.branch IS NOT NULL AND BTRIM(obs.branch) <> ''
 AND obs.revision_id=src.active_revision_id
ORDER BY obs.user_id,obs.source_id,obs.branch,obs.last_seen_at DESC,obs.observation_id DESC
ON CONFLICT(user_id,source_id,branch) DO UPDATE SET
 revision_id=EXCLUDED.revision_id,
 updated_at=GREATEST(codelocal_knowledge_branch_heads.updated_at,EXCLUDED.updated_at);
ALTER TABLE codelocal_knowledge_conflicts ADD COLUMN IF NOT EXISTS branch TEXT;
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_conflicts_branch
 ON codelocal_knowledge_conflicts(user_id,source_id,branch,updated_at DESC) WHERE branch IS NOT NULL;
`},
	}
	migrations = append(migrations, knowledgeV2SchemaMigrations()...)
	migrations = append(migrations, accountSchemaMigrations()...)
	if err := validateSchemaMigrationPlan(migrations); err != nil {
		return err
	}
	targetVersion := migrations[len(migrations)-1].version
	var currentVersion, appliedCount int
	if err := conn.QueryRow(ctx, `SELECT COALESCE(MAX(version),0),COUNT(*) FROM codelocal_schema_migrations`).Scan(&currentVersion, &appliedCount); err != nil {
		return err
	}
	if err := validateDatabaseSchemaHistory(currentVersion, appliedCount, targetVersion); err != nil {
		return err
	}
	for _, migration := range migrations {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM codelocal_schema_migrations WHERE version=$1)`, migration.version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		if nonTransactionalMigrationVersions[migration.version] {
			if _, err := conn.Exec(ctx, migration.sql); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, `INSERT INTO codelocal_schema_migrations(version,applied_at) VALUES($1,$2) ON CONFLICT(version) DO NOTHING`, migration.version, time.Now().UnixMilli()); err != nil {
				return err
			}
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, migration.sql); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO codelocal_schema_migrations(version,applied_at) VALUES($1,$2)`, migration.version, time.Now().UnixMilli())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	var finalVersion, finalCount int
	if err := conn.QueryRow(ctx, `SELECT COALESCE(MAX(version),0),COUNT(*) FROM codelocal_schema_migrations`).Scan(&finalVersion, &finalCount); err != nil {
		return err
	}
	if err := validateDatabaseSchemaHistory(finalVersion, finalCount, targetVersion); err != nil {
		return err
	}
	if finalVersion != targetVersion || finalCount != targetVersion {
		return fmt.Errorf("schema migration incomplete: currentVersion=%d appliedCount=%d targetVersion=%d", finalVersion, finalCount, targetVersion)
	}
	return nil
}

func RandomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func HashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func EqualSecretHash(actual, expected string) bool {
	a, errA := hex.DecodeString(actual)
	b, errB := hex.DecodeString(expected)
	return errA == nil && errB == nil && len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func (s *Store) invalidateDeviceCache(keys ...string) {
	if len(keys) == 0 || s.Redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Redis.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("device cache invalidation failed", "error", err, "keyCount", len(keys))
	}
}

func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

const defaultAdminEmail = "monglv36@gmail.com"

func AdminEmails() []string {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_ADMIN_EMAILS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("CODELOCAL_ADMIN_EMAIL"))
	}
	if raw == "" {
		raw = defaultAdminEmail
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		email := normalizeEmail(part)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	if len(out) == 0 {
		return []string{defaultAdminEmail}
	}
	return out
}

func PrimaryAdminEmail() string { return AdminEmails()[0] }

func IsAdminEmail(email string) bool {
	email = normalizeEmail(email)
	for _, allowed := range AdminEmails() {
		if email == allowed {
			return true
		}
	}
	return false
}

func NormalizeReferralCode(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func ValidReferralCode(value string) bool {
	value = NormalizeReferralCode(value)
	if value == "MMON" {
		return true
	}
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func RandomReferralCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		n := uint64(time.Now().UnixNano())
		for i := range buf {
			buf[i] = alphabet[n%uint64(len(alphabet))]
			n /= uint64(len(alphabet))
		}
		return string(buf)
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

func (s *Store) RateLimit(ctx context.Context, scope, identifier string, limit, windowSeconds int) (allowed bool, count, retry int, err error) {
	safeScope := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r) {
			return r
		}
		return '-'
	}, scope)
	if safeScope == "" {
		safeScope = "default"
	}
	if len(safeScope) > 80 {
		safeScope = safeScope[:80]
	}
	subject := HashSecret(identifier)
	key := "codelocal:rate:" + safeScope + ":" + subject[:32]
	script := `local count=redis.call('INCR',KEYS[1]); if count==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]) end; return count`
	count64, err := s.Redis.Eval(ctx, script, []string{key}, maxInt(1, windowSeconds)).Int64()
	if err != nil {
		return false, 0, 0, err
	}
	count = int(count64)
	allowed = count <= maxInt(1, limit)
	if !allowed {
		ttl, ttlErr := s.Redis.TTL(ctx, key).Result()
		if ttlErr == nil {
			retry = maxInt(1, int(ttl.Seconds()))
		} else {
			retry = 1
		}
	}
	return allowed, count, retry, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, passwordSalt, referredByCode string) (User, error) {
	email = normalizeEmail(email)
	referredByCode = NormalizeReferralCode(referredByCode)
	if !ValidReferralCode(referredByCode) {
		return User{}, errors.New("REFERRAL_REQUIRED")
	}

	primaryAdmin := email == PrimaryAdminEmail()
	if referredByCode == "MMON" {
		// MMON is a reserved legacy root marker. Only the primary admin email may
		// use it to bootstrap an account; normal users must have a real member code.
		if !primaryAdmin {
			return User{}, errors.New("REFERRAL_INVALID")
		}
	} else {
		inviter, err := s.UserByReferralCode(ctx, referredByCode)
		if err != nil {
			return User{}, err
		}
		if inviter == nil {
			return User{}, errors.New("REFERRAL_INVALID")
		}
	}

	for attempt := 0; attempt < 10; attempt++ {
		ownCode := RandomReferralCode()
		user := User{ID: RandomHex(16), Email: email, PasswordHash: passwordHash, PasswordSalt: passwordSalt, SecurityVersion: 1, ReferralCode: ownCode, ReferredByCode: referredByCode, CreatedAt: time.Now().UnixMilli()}
		_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_users(id,email,password_hash,password_salt,referral_code,referred_by_code,created_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7)`, user.ID, user.Email, user.PasswordHash, user.PasswordSalt, user.ReferralCode, user.ReferredByCode, user.CreatedAt)
		if err == nil {
			return user, nil
		}
		if strings.Contains(err.Error(), "23505") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			if existing, lookupErr := s.UserByEmail(ctx, email); lookupErr == nil && existing != nil {
				return User{}, errors.New("EMAIL_ALREADY_REGISTERED")
			}
			continue
		}
		return User{}, err
	}
	return User{}, errors.New("REFERRAL_CODE_GENERATION_FAILED")
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	var referredBy *string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.PasswordSalt, &u.PasswordChangedAt, &u.SecurityVersion, &u.ReferralCode, &referredBy, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if referredBy != nil {
		u.ReferredByCode = *referredBy
	}
	return &u, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,password_changed_at,security_version,referral_code,referred_by_code,created_at FROM codelocal_users WHERE email=$1`, normalizeEmail(email)))
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,password_changed_at,security_version,referral_code,referred_by_code,created_at FROM codelocal_users WHERE id=$1`, id))
}

func (s *Store) UserByReferralCode(ctx context.Context, code string) (*User, error) {
	code = NormalizeReferralCode(code)
	if !ValidReferralCode(code) {
		return nil, nil
	}
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,password_changed_at,security_version,referral_code,referred_by_code,created_at FROM codelocal_users WHERE UPPER(referral_code)=$1`, code))
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID, passwordHash, passwordSalt string, changedAt int64) error {
	result, err := s.DB.Exec(ctx, `UPDATE codelocal_users SET password_hash=$2,password_salt=$3,password_changed_at=$4,security_version=security_version+1 WHERE id=$1`, userID, passwordHash, passwordSalt, changedAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("USER_NOT_FOUND")
	}
	_ = s.ClearUserSecurityCache(ctx, userID)
	return nil
}

func (s *Store) CreateSession(ctx context.Context, userID, csrf string, ttl time.Duration) (string, error) {
	return s.CreateSessionWithSecurity(ctx, userID, csrf, ttl, SecuritySignal{})
}

func (s *Store) CreateSessionWithSecurity(ctx context.Context, userID, csrf string, ttl time.Duration, signal SecuritySignal) (string, error) {
	id := RandomHex(40)
	state := SessionState{UserID: userID, CSRF: csrf, CreatedAt: time.Now().UnixMilli()}
	if signal.DeviceHash != "" || signal.AgentHash != "" || signal.NetworkHash != "" {
		state.Security = &signal
	}
	data, _ := json.Marshal(state)
	return id, s.Redis.Set(ctx, "codelocal:session:"+id, data, ttl).Err()
}

func (s *Store) UpdateSessionState(ctx context.Context, id string, state SessionState) error {
	if id == "" {
		return nil
	}
	key := "codelocal:session:" + id
	ttl, err := s.Redis.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.Redis.Set(ctx, key, data, ttl).Err()
}

func (s *Store) ReadSessionState(ctx context.Context, id string) (SessionState, bool, error) {
	if id == "" {
		return SessionState{}, false, nil
	}
	raw, err := s.Redis.Get(ctx, "codelocal:session:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return SessionState{}, false, nil
	}
	if err != nil {
		return SessionState{}, false, err
	}
	var state SessionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return SessionState{}, false, nil
	}
	return state, state.UserID != "" && state.CSRF != "", nil
}

func (s *Store) ReadSession(ctx context.Context, id string) (userID, csrf string, ok bool, err error) {
	state, ok, err := s.ReadSessionState(ctx, id)
	return state.UserID, state.CSRF, ok, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	return s.Redis.Del(ctx, "codelocal:session:"+id).Err()
}

func (s *Store) CreatePairing(ctx context.Context, deviceID, deviceName string, ttl time.Duration) (Pairing, error) {
	codeBytes := make([]byte, 4)
	_, _ = rand.Read(codeBytes)
	code := int(codeBytes[0])<<16 | int(codeBytes[1])<<8 | int(codeBytes[2])
	code = 100000 + code%900000
	pairing := Pairing{PairingID: RandomHex(16), Code: fmt.Sprintf("%06d", code), DeviceID: deviceID, DeviceName: deviceName, CreatedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(ttl).UnixMilli()}
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_pairings(pairing_id,code,device_id,device_name,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, pairing.PairingID, pairing.Code, pairing.DeviceID, pairing.DeviceName, pairing.CreatedAt, pairing.ExpiresAt)
	return pairing, err
}

func scanPairing(row pgx.Row) (*Pairing, error) {
	var p Pairing
	var userID *string
	var approved, claimed *int64
	err := row.Scan(&p.PairingID, &p.Code, &p.DeviceID, &p.DeviceName, &userID, &p.CreatedAt, &p.ExpiresAt, &approved, &claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if userID != nil {
		p.UserID = *userID
	}
	if approved != nil {
		p.ApprovedAt = *approved
	}
	if claimed != nil {
		p.ClaimedAt = *claimed
	}
	return &p, nil
}

func (s *Store) GetPairing(ctx context.Context, id string) (*Pairing, error) {
	return scanPairing(s.DB.QueryRow(ctx, `SELECT pairing_id,code,device_id,device_name,user_id,created_at,expires_at,approved_at,claimed_at FROM codelocal_pairings WHERE pairing_id=$1`, id))
}

func (s *Store) ApprovePairing(ctx context.Context, id, code, userID string) (*Pairing, error) {
	now := time.Now().UnixMilli()
	return scanPairing(s.DB.QueryRow(ctx, `UPDATE codelocal_pairings SET user_id=$3,approved_at=$4 WHERE pairing_id=$1 AND code=$2 AND expires_at>$4 AND approved_at IS NULL AND claimed_at IS NULL AND user_id IS NULL RETURNING pairing_id,code,device_id,device_name,user_id,created_at,expires_at,approved_at,claimed_at`, id, code, userID, now))
}

func (s *Store) ClaimPairing(ctx context.Context, id, code, credentialID, secretHash, publicKey string) (*Device, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var p Pairing
	var userID string
	err = tx.QueryRow(ctx, `SELECT pairing_id,code,device_id,device_name,user_id,created_at,expires_at FROM codelocal_pairings WHERE pairing_id=$1 AND code=$2 AND expires_at>$3 AND approved_at IS NOT NULL AND claimed_at IS NULL FOR UPDATE`, id, code, time.Now().UnixMilli()).Scan(&p.PairingID, &p.Code, &p.DeviceID, &p.DeviceName, &userID, &p.CreatedAt, &p.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// A claim may have committed even when its HTTP response was lost. Allow a
		// retry only when the caller presents the exact credential ID and secret
		// hash that became current for this pairing's device.
		claimedErr := tx.QueryRow(ctx, `SELECT pairing_id,code,device_id,device_name,user_id,created_at,expires_at FROM codelocal_pairings WHERE pairing_id=$1 AND code=$2 AND approved_at IS NOT NULL AND claimed_at IS NOT NULL`, id, code).Scan(&p.PairingID, &p.Code, &p.DeviceID, &p.DeviceName, &userID, &p.CreatedAt, &p.ExpiresAt)
		if errors.Is(claimedErr, pgx.ErrNoRows) {
			return nil, nil
		}
		if claimedErr != nil {
			return nil, claimedErr
		}
		var d Device
		var revoked *int64
		deviceErr := tx.QueryRow(ctx, `SELECT credential_id,user_id,device_id,device_name,COALESCE(public_key,''),secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE user_id=$1 AND device_id=$2 AND credential_id=$3`, userID, p.DeviceID, credentialID).Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.PublicKey, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked)
		if errors.Is(deviceErr, pgx.ErrNoRows) {
			return nil, nil
		}
		if deviceErr != nil {
			return nil, deviceErr
		}
		if revoked != nil || !EqualSecretHash(d.SecretHash, secretHash) || d.PublicKey != strings.TrimSpace(publicKey) {
			return nil, nil
		}
		return &d, nil
	}
	if err != nil {
		return nil, err
	}
	previousCredentialID := ""
	if existingErr := tx.QueryRow(ctx, `SELECT credential_id FROM codelocal_devices WHERE user_id=$1 AND device_id=$2 FOR UPDATE`, userID, p.DeviceID).Scan(&previousCredentialID); existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
		return nil, existingErr
	}
	now := time.Now().UnixMilli()
	if _, err = tx.Exec(ctx, `UPDATE codelocal_pairings SET claimed_at=$2 WHERE pairing_id=$1`, id, now); err != nil {
		return nil, err
	}
	publicKey = strings.TrimSpace(publicKey)
	if _, err = tx.Exec(ctx, `INSERT INTO codelocal_devices(credential_id,user_id,device_id,device_name,public_key,secret_hash,created_at,last_seen_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$7) ON CONFLICT(user_id,device_id) DO UPDATE SET credential_id=EXCLUDED.credential_id,device_name=EXCLUDED.device_name,public_key=EXCLUDED.public_key,secret_hash=EXCLUDED.secret_hash,created_at=EXCLUDED.created_at,last_seen_at=EXCLUDED.last_seen_at,revoked_at=NULL`, credentialID, userID, p.DeviceID, p.DeviceName, publicKey, secretHash, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	keys := []string{"codelocal:device:" + credentialID, "codelocal:device-touch:" + credentialID}
	if previousCredentialID != "" && previousCredentialID != credentialID {
		keys = append(keys, "codelocal:device:"+previousCredentialID, "codelocal:device-touch:"+previousCredentialID)
		_ = s.ClearSecurityState(ctx, "credential", previousCredentialID)
		_ = s.ClearDeviceNonceState(ctx, previousCredentialID)
	}
	s.invalidateDeviceCache(keys...)
	return &Device{CredentialID: credentialID, PreviousCredentialID: previousCredentialID, UserID: userID, DeviceID: p.DeviceID, DeviceName: p.DeviceName, PublicKey: publicKey, SecretHash: secretHash, CreatedAt: now, LastSeenAt: now}, nil
}

func (s *Store) AuthenticateDevice(ctx context.Context, credentialID, secretHash string) (*Device, error) {
	if credentialID == "" || secretHash == "" {
		return nil, nil
	}
	cacheKey := "codelocal:device:" + credentialID
	if raw, err := s.Redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var cached Device
		if json.Unmarshal(raw, &cached) == nil && cached.RevokedAt == 0 && EqualSecretHash(cached.SecretHash, secretHash) {
			cached.LastSeenAt = time.Now().UnixMilli()
			return &cached, nil
		}
	}
	var d Device
	var revoked *int64
	err := s.DB.QueryRow(ctx, `SELECT credential_id,user_id,device_id,device_name,COALESCE(public_key,''),secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE credential_id=$1`, credentialID).Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.PublicKey, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revoked != nil || !EqualSecretHash(d.SecretHash, secretHash) {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	d.LastSeenAt = now
	payload, _ := json.Marshal(d)
	_ = s.Redis.Set(ctx, cacheKey, payload, 60*time.Second).Err()
	lastPersistKey := "codelocal:device-touch:" + credentialID
	if ok, _ := s.Redis.SetNX(ctx, lastPersistKey, "1", 60*time.Second).Result(); ok {
		go func() {
			_, _ = s.DB.Exec(context.Background(), `UPDATE codelocal_devices SET last_seen_at=$2 WHERE credential_id=$1`, credentialID, now)
		}()
	}
	return &d, nil
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.DB.Query(ctx, `SELECT credential_id,user_id,device_id,device_name,COALESCE(public_key,''),secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE user_id=$1 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var revoked *int64
		if err := rows.Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.PublicKey, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked); err != nil {
			return nil, err
		}
		if revoked != nil {
			d.RevokedAt = *revoked
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ActiveCredentialIDs(ctx context.Context, credentialIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(credentialIDs))
	if len(credentialIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT credential_id FROM codelocal_devices WHERE credential_id = ANY($1::text[]) AND revoked_at IS NULL`, credentialIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var credentialID string
		if err := rows.Scan(&credentialID); err != nil {
			return nil, err
		}
		out[credentialID] = true
	}
	return out, rows.Err()
}

func (s *Store) RevokeDevice(ctx context.Context, userID, credentialID string) (bool, error) {
	result, err := s.DB.Exec(ctx, `UPDATE codelocal_devices SET revoked_at=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL`, userID, credentialID, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	s.invalidateDeviceCache("codelocal:device:"+credentialID, "codelocal:device-touch:"+credentialID)
	_ = s.ClearSecurityState(ctx, "credential", credentialID)
	_ = s.ClearDeviceNonceState(ctx, credentialID)
	return result.RowsAffected() == 1, nil
}

func (s *Store) RenameDevice(ctx context.Context, userID, credentialID, name string) (bool, error) {
	result, err := s.DB.Exec(ctx, `UPDATE codelocal_devices SET device_name=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL`, userID, credentialID, name)
	if err != nil {
		return false, err
	}
	s.invalidateDeviceCache("codelocal:device:" + credentialID)
	return result.RowsAffected() == 1, nil
}

func (s *Store) UpsertWorkspace(ctx context.Context, w Workspace) error {
	now := time.Now().UnixMilli()
	caps, _ := json.Marshal(w.Capabilities)
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_workspaces(user_id,device_id,workspace_id,workspace_name,project_root,protocol_version,capabilities,created_at,last_seen_at) VALUES($1,$2,$3,$4,NULL,$5,$6,$7,$7) ON CONFLICT(user_id,device_id,workspace_id) DO UPDATE SET workspace_name=EXCLUDED.workspace_name,project_root=NULL,protocol_version=EXCLUDED.protocol_version,capabilities=EXCLUDED.capabilities,last_seen_at=EXCLUDED.last_seen_at`, w.UserID, w.DeviceID, w.WorkspaceID, w.WorkspaceName, w.ProtocolVersion, caps, now)
	return err
}

func (s *Store) ListWorkspaceRecords(ctx context.Context, userID string) ([]Workspace, error) {
	rows, err := s.DB.Query(ctx, `
SELECT w.user_id,w.device_id,w.workspace_id,w.workspace_name,w.project_root,w.protocol_version,w.capabilities,w.created_at,w.last_seen_at,
 COALESCE(wp.project_id,''),COALESCE(p.name,''),COALESCE(wp.source,''),COALESCE(wp.confidence,0)
FROM codelocal_workspaces w
LEFT JOIN codelocal_workspace_projects wp ON wp.user_id=w.user_id AND wp.device_id=w.device_id AND wp.workspace_id=w.workspace_id
LEFT JOIN codelocal_projects p ON p.user_id=wp.user_id AND p.project_id=wp.project_id
WHERE w.user_id=$1
ORDER BY w.last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		var projectRoot *string
		var protocolVersion *int
		var caps []byte
		if err := rows.Scan(&w.UserID, &w.DeviceID, &w.WorkspaceID, &w.WorkspaceName, &projectRoot, &protocolVersion, &caps, &w.CreatedAt, &w.LastSeenAt, &w.ProjectID, &w.ProjectName, &w.ProjectSource, &w.ProjectConfidence); err != nil {
			return nil, err
		}
		if projectRoot != nil {
			w.ProjectRoot = *projectRoot
		}
		if protocolVersion != nil {
			w.ProtocolVersion = *protocolVersion
		}
		_ = json.Unmarshal(caps, &w.Capabilities)
		if w.Capabilities == nil {
			w.Capabilities = map[string]any{}
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) ReconcileWorkspaces(ctx context.Context, userID, deviceID string, authorized []string) ([]string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT workspace_id FROM codelocal_workspaces WHERE user_id=$1 AND device_id=$2`, userID, deviceID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, id := range authorized {
		allowed[id] = struct{}{}
	}
	removed := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := allowed[id]; !ok {
			removed = append(removed, id)
		}
	}
	rows.Close()
	if len(removed) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM codelocal_workspaces WHERE user_id=$1 AND device_id=$2 AND workspace_id = ANY($3)`, userID, deviceID, removed); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return removed, nil
}

func (s *Store) CreateOAuthClient(ctx context.Context, client OAuthClient) (OAuthClient, error) {
	client.CreatedAt = time.Now().UnixMilli()
	raw, _ := json.Marshal(client.RedirectURIs)
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_oauth_clients(client_id,redirect_uris,client_name,created_at) VALUES($1,$2,$3,$4)`, client.ClientID, raw, nullIfEmpty(client.ClientName), client.CreatedAt)
	return client, err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) OAuthClient(ctx context.Context, clientID string) (*OAuthClient, error) {
	var c OAuthClient
	var raw []byte
	var name *string
	err := s.DB.QueryRow(ctx, `SELECT client_id,redirect_uris,client_name,created_at FROM codelocal_oauth_clients WHERE client_id=$1`, clientID).Scan(&c.ClientID, &raw, &name, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &c.RedirectURIs)
	if name != nil {
		c.ClientName = *name
	}
	return &c, nil
}

func (s *Store) PutOAuthCode(ctx context.Context, code OAuthCode) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_oauth_codes(code,user_id,client_id,redirect_uri,code_challenge,resource,scope,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, code.Code, code.UserID, code.ClientID, code.RedirectURI, code.CodeChallenge, code.Resource, code.Scope, code.ExpiresAt)
	return err
}

func (s *Store) ConsumeOAuthCode(ctx context.Context, code string) (*OAuthCode, error) {
	var out OAuthCode
	err := s.DB.QueryRow(ctx, `DELETE FROM codelocal_oauth_codes WHERE code=$1 RETURNING code,user_id,client_id,redirect_uri,code_challenge,resource,scope,expires_at`, code).Scan(&out.Code, &out.UserID, &out.ClientID, &out.RedirectURI, &out.CodeChallenge, &out.Resource, &out.Scope, &out.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) Audit(_ AuditEvent) {
	// Cloud audit storage is disabled. Keep call sites as no-ops so runtime,
	// auth and workspace behavior stay unchanged without persisting metadata.
}

func parseUsageInt64(value string) int64 {
	n, _ := strconv.ParseInt(value, 10, 64)
	return n
}

func usageBucketStart(createdAt int64) int64 {
	return (createdAt / 3600000) * 3600000
}

func usageDayStart(createdAt int64) int64 {
	const dayMs = int64(24 * time.Hour / time.Millisecond)
	return (createdAt / dayMs) * dayMs
}

func usageWindowKey(userID, bucket string, startedAt int64) string {
	return "codelocal:usage:" + bucket + ":" + userID + ":" + strconv.FormatInt(startedAt, 10)
}

func mcpActiveKey(userID string) string { return "codelocal:user:mcp-active:" + userID }

func (s *Store) TouchUserMCPActive(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	return s.Redis.Set(ctx, mcpActiveKey(userID), time.Now().UnixMilli(), 5*time.Minute).Err()
}

func (s *Store) IsUserMCPActive(ctx context.Context, userID string) (bool, error) {
	n, err := s.Redis.Exists(ctx, mcpActiveKey(userID)).Result()
	return n > 0, err
}

func (s *Store) UserMCPActiveMap(ctx context.Context, userIDs []string) (map[string]bool, error) {
	commands := make(map[string]*redis.IntCmd, len(userIDs))
	_, err := s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, userID := range userIDs {
			if userID == "" {
				continue
			}
			commands[userID] = pipe.Exists(ctx, mcpActiveKey(userID))
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make(map[string]bool, len(commands))
	for userID, command := range commands {
		out[userID] = command.Val() > 0
	}
	return out, nil
}

func (s *Store) RecordMCPUsage(ctx context.Context, event MCPUsageEvent) error {
	_ = ctx
	if strings.TrimSpace(event.UserID) == "" {
		return nil
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	if event.Calls <= 0 {
		event.Calls = 1
	}
	select {
	case s.usageQ <- event:
		return nil
	default:
		// Usage is approximate telemetry, never a reason to delay or fail a user
		// tool call. Keep a visible counter instead of falling back to synchronous
		// PostgreSQL writes on the request path.
		dropped := s.usageDropped.Add(1)
		if dropped == 1 || dropped%1000 == 0 {
			slog.Warn("MCP usage local queue full; telemetry event dropped", "dropped", dropped)
		}
		return nil
	}
}

type usageAggregateKey struct {
	UserID      string
	BucketStart int64
}

type queuedUsageAggregate struct {
	BatchID string
	Event   MCPUsageEvent
}

func mergeMCPUsage(target MCPUsageEvent, event MCPUsageEvent) MCPUsageEvent {
	if target.UserID == "" {
		target = event
		target.Calls = 0
		target.InputBytes = 0
		target.OutputBytes = 0
		target.InputTokensEst = 0
		target.OutputTokensEst = 0
	}
	if event.CreatedAt > target.CreatedAt {
		target.CreatedAt = event.CreatedAt
	}
	target.Calls += event.Calls
	target.InputBytes += event.InputBytes
	target.OutputBytes += event.OutputBytes
	target.InputTokensEst += event.InputTokensEst
	target.OutputTokensEst += event.OutputTokensEst
	return target
}

func (s *Store) ensureUsageStreamGroup(ctx context.Context) error {
	err := s.Redis.XGroupCreateMkStream(ctx, usageStreamKey, usageStreamGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (s *Store) usageStreamProducer() {
	defer s.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	pending := map[usageAggregateKey]queuedUsageAggregate{}
	lastActivityTouch := map[string]time.Time{}
	add := func(event MCPUsageEvent) {
		if event.Calls <= 0 {
			event.Calls = 1
		}
		bucket := usageBucketStart(event.CreatedAt)
		key := usageAggregateKey{UserID: event.UserID, BucketStart: bucket}
		current := pending[key]
		if current.BatchID == "" {
			current.BatchID = RandomHex(16)
		}
		current.Event = mergeMCPUsage(current.Event, event)
		pending[key] = current
	}
	flush := func(timeout time.Duration) bool {
		if len(pending) == 0 {
			return true
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		pipe := s.Redis.Pipeline()
		now := time.Now()
		touched := []string{}
		maxLen := int64(envInt("CODELOCAL_USAGE_STREAM_MAXLEN", 1_000_000))
		for _, aggregate := range pending {
			raw, err := json.Marshal(aggregate.Event)
			if err != nil {
				continue
			}
			pipe.XAdd(ctx, &redis.XAddArgs{Stream: usageStreamKey, MaxLen: maxLen, Approx: true, Values: map[string]any{"batchId": aggregate.BatchID, "event": string(raw)}})
			if last := lastActivityTouch[aggregate.Event.UserID]; last.IsZero() || now.Sub(last) >= time.Minute {
				pipe.Set(ctx, mcpActiveKey(aggregate.Event.UserID), now.UnixMilli(), 5*time.Minute)
				touched = append(touched, aggregate.Event.UserID)
			}
		}
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Error("MCP usage Redis stream publish failed", "error", err, "aggregateCount", len(pending))
			return false
		}
		for _, userID := range touched {
			lastActivityTouch[userID] = now
		}
		clear(pending)
		return true
	}
	for {
		select {
		case <-s.ctx.Done():
			for {
				select {
				case event := <-s.usageQ:
					add(event)
				default:
					_ = flush(3 * time.Second)
					return
				}
			}
		case event := <-s.usageQ:
			add(event)
			if len(pending) >= 256 {
				_ = flush(3 * time.Second)
			}
		case <-ticker.C:
			_ = flush(3 * time.Second)
		}
	}
}

func (s *Store) refreshUsageConsumerLease(ctx context.Context) (bool, error) {
	const lease = 15 * time.Second
	script := `if redis.call('GET',KEYS[1])==ARGV[1] then redis.call('PEXPIRE',KEYS[1],ARGV[2]); return 1 end; if redis.call('SET',KEYS[1],ARGV[1],'NX','PX',ARGV[2]) then return 1 end; return 0`
	value, err := s.Redis.Eval(ctx, script, []string{usageConsumerLockKey}, s.usageConsumerID, lease.Milliseconds()).Int()
	return value == 1, err
}

func (s *Store) releaseUsageConsumerLease() {
	script := `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.Redis.Eval(ctx, script, []string{usageConsumerLockKey}, s.usageConsumerID).Err()
}

func usageEventFromStream(message redis.XMessage) (string, MCPUsageEvent, error) {
	batchID := strings.TrimSpace(fmt.Sprint(message.Values["batchId"]))
	if batchID == "" || batchID == "<nil>" {
		batchID = message.ID
	}
	var event MCPUsageEvent
	if err := json.Unmarshal([]byte(fmt.Sprint(message.Values["event"])), &event); err != nil {
		return batchID, event, err
	}
	if event.UserID == "" {
		return batchID, event, errors.New("usage event missing userId")
	}
	if event.Calls <= 0 {
		event.Calls = 1
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	return batchID, event, nil
}

func (s *Store) persistMCPUsageMessages(ctx context.Context, messages []redis.XMessage) ([]string, error) {
	ackIDs := make([]string, 0, len(messages))
	type queued struct {
		messageID string
		batchID   string
		event     MCPUsageEvent
	}
	valid := make([]queued, 0, len(messages))
	for _, message := range messages {
		batchID, event, err := usageEventFromStream(message)
		if err != nil {
			slog.Warn("discarding invalid MCP usage stream event", "messageId", message.ID, "error", err)
			ackIDs = append(ackIDs, message.ID)
			continue
		}
		valid = append(valid, queued{messageID: message.ID, batchID: batchID, event: event})
	}
	if len(valid) == 0 {
		return ackIDs, nil
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	dbAggregates := map[string]MCPUsageEvent{}
	windowAggregates := map[usageAggregateKey]MCPUsageEvent{}
	for _, item := range valid {
		tag, err := tx.Exec(ctx, `INSERT INTO codelocal_mcp_usage_batches(id,processed_at) VALUES($1,$2) ON CONFLICT(id) DO NOTHING`, item.batchID, time.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		ackIDs = append(ackIDs, item.messageID)
		if tag.RowsAffected() == 0 {
			continue
		}
		dbAggregates[item.event.UserID] = mergeMCPUsage(dbAggregates[item.event.UserID], item.event)
		key := usageAggregateKey{UserID: item.event.UserID, BucketStart: usageBucketStart(item.event.CreatedAt)}
		windowAggregates[key] = mergeMCPUsage(windowAggregates[key], item.event)
	}
	for _, event := range dbAggregates {
		if _, err := tx.Exec(ctx, `INSERT INTO codelocal_mcp_usage(user_id,calls,input_bytes,output_bytes,input_tokens_est,output_tokens_est,last_used_at)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(user_id) DO UPDATE SET
 calls=codelocal_mcp_usage.calls+EXCLUDED.calls,
 input_bytes=codelocal_mcp_usage.input_bytes+EXCLUDED.input_bytes,
 output_bytes=codelocal_mcp_usage.output_bytes+EXCLUDED.output_bytes,
 input_tokens_est=codelocal_mcp_usage.input_tokens_est+EXCLUDED.input_tokens_est,
 output_tokens_est=codelocal_mcp_usage.output_tokens_est+EXCLUDED.output_tokens_est,
 last_used_at=GREATEST(codelocal_mcp_usage.last_used_at,EXCLUDED.last_used_at)`, event.UserID, event.Calls, event.InputBytes, event.OutputBytes, event.InputTokensEst, event.OutputTokensEst, event.CreatedAt); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if len(windowAggregates) > 0 {
		pipe := s.Redis.Pipeline()
		for _, event := range windowAggregates {
			hourKey := usageWindowKey(event.UserID, "h", usageBucketStart(event.CreatedAt))
			dayKey := usageWindowKey(event.UserID, "d", usageDayStart(event.CreatedAt))
			for _, key := range []string{hourKey, dayKey} {
				pipe.HIncrBy(ctx, key, "calls", event.Calls)
				pipe.HIncrBy(ctx, key, "input_bytes", int64(event.InputBytes))
				pipe.HIncrBy(ctx, key, "output_bytes", int64(event.OutputBytes))
				pipe.HIncrBy(ctx, key, "input_tokens", int64(event.InputTokensEst))
				pipe.HIncrBy(ctx, key, "output_tokens", int64(event.OutputTokensEst))
			}
			pipe.Expire(ctx, hourKey, 49*time.Hour)
			pipe.Expire(ctx, dayKey, 35*24*time.Hour)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Warn("MCP rolling usage batch update failed", "error", err)
		}
	}
	return ackIDs, nil
}

func (s *Store) usageStreamConsumer() {
	defer s.wg.Done()
	defer s.releaseUsageConsumerLease()
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		leaseCtx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
		leader, err := s.refreshUsageConsumerLease(leaseCtx)
		cancel()
		if err != nil || !leader {
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}

		readCtx, readCancel := context.WithTimeout(s.ctx, 3*time.Second)
		messages, _, claimErr := s.Redis.XAutoClaim(readCtx, &redis.XAutoClaimArgs{Stream: usageStreamKey, Group: usageStreamGroup, Consumer: s.usageConsumerID, MinIdle: 20 * time.Second, Start: "0-0", Count: 128}).Result()
		if claimErr != nil && !errors.Is(claimErr, redis.Nil) && !errors.Is(claimErr, context.Canceled) {
			slog.Warn("MCP usage pending claim failed", "error", claimErr)
		}
		if len(messages) == 0 && (claimErr == nil || errors.Is(claimErr, redis.Nil)) {
			streams, readErr := s.Redis.XReadGroup(readCtx, &redis.XReadGroupArgs{Group: usageStreamGroup, Consumer: s.usageConsumerID, Streams: []string{usageStreamKey, ">"}, Count: 128, Block: time.Second}).Result()
			if readErr != nil && !errors.Is(readErr, redis.Nil) && !errors.Is(readErr, context.Canceled) {
				slog.Warn("MCP usage stream read failed", "error", readErr)
			}
			for _, stream := range streams {
				messages = append(messages, stream.Messages...)
			}
		}
		readCancel()
		if len(messages) == 0 {
			continue
		}
		persistCtx, persistCancel := context.WithTimeout(s.ctx, 8*time.Second)
		ackIDs, persistErr := s.persistMCPUsageMessages(persistCtx, messages)
		persistCancel()
		if persistErr != nil {
			slog.Error("MCP usage stream persistence failed", "error", persistErr, "messageCount", len(messages))
			continue
		}
		if len(ackIDs) > 0 {
			ackCtx, ackCancel := context.WithTimeout(s.ctx, 2*time.Second)
			if err := s.Redis.XAck(ackCtx, usageStreamKey, usageStreamGroup, ackIDs...).Err(); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("MCP usage stream ack failed", "error", err, "messageCount", len(ackIDs))
			}
			ackCancel()
		}
	}
}

func (s *Store) MCPUsageSummary(ctx context.Context, userID string, since int64) (MCPUsageSummary, error) {
	var out MCPUsageSummary
	if since <= 0 {
		err := s.DB.QueryRow(ctx, `SELECT calls,input_bytes,output_bytes,input_tokens_est,output_tokens_est FROM codelocal_mcp_usage WHERE user_id=$1`, userID).Scan(&out.Calls, &out.InputBytes, &out.OutputBytes, &out.InputTokensEst, &out.OutputTokensEst)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		out.TotalTokensEst = out.InputTokensEst + out.OutputTokensEst
		return out, err
	}

	now := time.Now().UnixMilli()
	useHourly := now-since <= int64(48*time.Hour/time.Millisecond)
	bucket := "d"
	step := int64(24 * time.Hour / time.Millisecond)
	start := usageDayStart(since)
	if useHourly {
		bucket = "h"
		step = int64(time.Hour / time.Millisecond)
		start = usageBucketStart(since)
	}
	keys := make([]string, 0, 32)
	for at := start; at <= now; at += step {
		keys = append(keys, usageWindowKey(userID, bucket, at))
	}
	commands := make([]*redis.MapStringStringCmd, 0, len(keys))
	_, err := s.Redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, key := range keys {
			commands = append(commands, pipe.HGetAll(ctx, key))
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return out, err
	}
	for _, command := range commands {
		values := command.Val()
		out.Calls += parseUsageInt64(values["calls"])
		out.InputBytes += parseUsageInt64(values["input_bytes"])
		out.OutputBytes += parseUsageInt64(values["output_bytes"])
		out.InputTokensEst += parseUsageInt64(values["input_tokens"])
		out.OutputTokensEst += parseUsageInt64(values["output_tokens"])
	}
	out.TotalTokensEst = out.InputTokensEst + out.OutputTokensEst
	return out, nil
}

type UsageLeaderboardUser struct {
	ID    string
	Email string
}

func (s *Store) ListUsageLeaderboardUsers(ctx context.Context, since int64) ([]UsageLeaderboardUser, error) {
	rows, err := s.DB.Query(ctx, `
SELECT u.id,u.email
FROM codelocal_users u
JOIN codelocal_mcp_usage m ON m.user_id=u.id
WHERE m.last_used_at >= $1
ORDER BY m.last_used_at DESC,u.id ASC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageLeaderboardUser{}
	for rows.Next() {
		var item UsageLeaderboardUser
		if err := rows.Scan(&item.ID, &item.Email); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListAdminUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := s.DB.Query(ctx, `
SELECT
 u.id,
 u.email,
 u.referral_code,
 COALESCE(u.referred_by_code,''),
 u.created_at,
 (SELECT COUNT(*) FROM codelocal_users child WHERE UPPER(child.referred_by_code)=UPPER(u.referral_code)) AS invite_count,
 COALESCE((SELECT MAX(d.last_seen_at) FROM codelocal_devices d WHERE d.user_id=u.id AND d.revoked_at IS NULL),0) AS last_device_seen_at,
 COALESCE((SELECT m.last_used_at FROM codelocal_mcp_usage m WHERE m.user_id=u.id),0) AS last_mcp_used_at
FROM codelocal_users u
ORDER BY u.created_at ASC,u.email ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var item AdminUser
		if err := rows.Scan(&item.ID, &item.Email, &item.ReferralCode, &item.ReferredByCode, &item.CreatedAt, &item.InviteCount, &item.LastDeviceSeenAt, &item.LastMCPUsedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListInvitedUsers(ctx context.Context, referralCode string) ([]AdminUser, error) {
	referralCode = NormalizeReferralCode(referralCode)
	if !ValidReferralCode(referralCode) {
		return []AdminUser{}, nil
	}
	rows, err := s.DB.Query(ctx, `
SELECT u.id,u.email,u.created_at
FROM codelocal_users u
WHERE UPPER(u.referred_by_code)=UPPER($1)
ORDER BY u.created_at DESC,u.email ASC`, referralCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var item AdminUser
		if err := rows.Scan(&item.ID, &item.Email, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RecentAudit(ctx context.Context, userID string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.DB.Query(ctx, `SELECT id,event,device_id,workspace_id,detail,created_at FROM codelocal_audit_logs WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, event string
		var deviceID, workspaceID *string
		var detail []byte
		var createdAt int64
		if err := rows.Scan(&id, &event, &deviceID, &workspaceID, &detail, &createdAt); err != nil {
			return nil, err
		}
		var decoded map[string]any
		_ = json.Unmarshal(detail, &decoded)
		out = append(out, map[string]any{"id": id, "event": event, "deviceId": deref(deviceID), "workspaceId": deref(workspaceID), "detail": decoded, "createdAt": createdAt})
	}
	return out, rows.Err()
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Store) retentionWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_pairings WHERE expires_at < $1 OR (claimed_at IS NOT NULL AND claimed_at < $2)`, time.Now().UnixMilli(), time.Now().Add(-24*time.Hour).UnixMilli())
			_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_oauth_codes WHERE expires_at < $1`, time.Now().UnixMilli())
			_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_mcp_usage_batches WHERE processed_at < $1`, time.Now().Add(-35*24*time.Hour).UnixMilli())
			if days := envInt("CODELOCAL_OUTBOX_RETENTION_DAYS", 14); days > 0 {
				cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
				_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_durable_outbox WHERE status IN ('processed','dead') AND updated_at < $1`, cutoff)
			}
			if days := envInt("CODELOCAL_KNOWLEDGE_V2_SHADOW_RETENTION_DAYS", 30); days > 0 {
				cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
				_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_knowledge_shadow_metrics WHERE last_sample_at < $1`, cutoff)
			}
			if days := envInt("CODELOCAL_AUDIT_RETENTION_DAYS", 90); days > 0 {
				_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_audit_logs WHERE created_at < $1`, time.Now().Add(-time.Duration(days)*24*time.Hour).UnixMilli())
			}
			cancel()
		}
	}
}
