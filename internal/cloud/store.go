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
	ID             string `json:"id"`
	Email          string `json:"email"`
	PasswordHash   string `json:"-"`
	PasswordSalt   string `json:"-"`
	ReferralCode   string `json:"referralCode"`
	ReferredByCode string `json:"referredByCode,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
}

type Device struct {
	CredentialID         string `json:"credentialId"`
	PreviousCredentialID string `json:"-"`
	UserID               string `json:"userId"`
	DeviceID             string `json:"deviceId"`
	DeviceName           string `json:"deviceName"`
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
	UserID          string         `json:"userId"`
	DeviceID        string         `json:"deviceId"`
	WorkspaceID     string         `json:"workspaceId"`
	WorkspaceName   string         `json:"workspaceName"`
	ProjectRoot     string         `json:"projectRoot,omitempty"`
	ProtocolVersion int            `json:"protocolVersion,omitempty"`
	Capabilities    map[string]any `json:"capabilities"`
	CreatedAt       int64          `json:"createdAt"`
	LastSeenAt      int64          `json:"lastSeenAt"`
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
	DB              *pgxpool.Pool
	Redis           *redis.Client
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	usageQ          chan MCPUsageEvent
	usageConsumerID string
	usageDropped    atomic.Uint64
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
	s := &Store{DB: db, Redis: rdb, ctx: storeCtx, cancel: cancel, usageQ: make(chan MCPUsageEvent, envInt("CODELOCAL_USAGE_LOCAL_QUEUE_SIZE", 8192)), usageConsumerID: RandomHex(12)}
	if err := s.Migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.ensureUsageStreamGroup(ctx); err != nil {
		s.Close()
		return nil, err
	}
	s.wg.Add(3)
	go s.usageStreamProducer()
	go s.usageStreamConsumer()
	go s.retentionWorker()
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

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS codelocal_schema_migrations (version INTEGER PRIMARY KEY, applied_at BIGINT NOT NULL)`); err != nil {
		return err
	}
	migrations := []struct {
		version int
		sql     string
	}{
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
	}
	for _, migration := range migrations {
		var exists bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM codelocal_schema_migrations WHERE version=$1)`, migration.version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		tx, err := s.DB.Begin(ctx)
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
		user := User{ID: RandomHex(16), Email: email, PasswordHash: passwordHash, PasswordSalt: passwordSalt, ReferralCode: ownCode, ReferredByCode: referredByCode, CreatedAt: time.Now().UnixMilli()}
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
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.PasswordSalt, &u.ReferralCode, &referredBy, &u.CreatedAt); err != nil {
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
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,referral_code,referred_by_code,created_at FROM codelocal_users WHERE email=$1`, normalizeEmail(email)))
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,referral_code,referred_by_code,created_at FROM codelocal_users WHERE id=$1`, id))
}

func (s *Store) UserByReferralCode(ctx context.Context, code string) (*User, error) {
	code = NormalizeReferralCode(code)
	if !ValidReferralCode(code) {
		return nil, nil
	}
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,referral_code,referred_by_code,created_at FROM codelocal_users WHERE UPPER(referral_code)=$1`, code))
}

func (s *Store) CreateSession(ctx context.Context, userID, csrf string, ttl time.Duration) (string, error) {
	id := RandomHex(40)
	data, _ := json.Marshal(map[string]string{"userId": userID, "csrf": csrf})
	return id, s.Redis.Set(ctx, "codelocal:session:"+id, data, ttl).Err()
}

func (s *Store) ReadSession(ctx context.Context, id string) (userID, csrf string, ok bool, err error) {
	if id == "" {
		return "", "", false, nil
	}
	raw, err := s.Redis.Get(ctx, "codelocal:session:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	var value map[string]string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", "", false, nil
	}
	return value["userId"], value["csrf"], value["userId"] != "" && value["csrf"] != "", nil
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

func (s *Store) ClaimPairing(ctx context.Context, id, code, credentialID, secretHash string) (*Device, error) {
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
		deviceErr := tx.QueryRow(ctx, `SELECT credential_id,user_id,device_id,device_name,secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE user_id=$1 AND device_id=$2 AND credential_id=$3`, userID, p.DeviceID, credentialID).Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked)
		if errors.Is(deviceErr, pgx.ErrNoRows) {
			return nil, nil
		}
		if deviceErr != nil {
			return nil, deviceErr
		}
		if revoked != nil || !EqualSecretHash(d.SecretHash, secretHash) {
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
	if _, err = tx.Exec(ctx, `INSERT INTO codelocal_devices(credential_id,user_id,device_id,device_name,secret_hash,created_at,last_seen_at) VALUES($1,$2,$3,$4,$5,$6,$6) ON CONFLICT(user_id,device_id) DO UPDATE SET credential_id=EXCLUDED.credential_id,device_name=EXCLUDED.device_name,secret_hash=EXCLUDED.secret_hash,created_at=EXCLUDED.created_at,last_seen_at=EXCLUDED.last_seen_at,revoked_at=NULL`, credentialID, userID, p.DeviceID, p.DeviceName, secretHash, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	keys := []string{"codelocal:device:" + credentialID, "codelocal:device-touch:" + credentialID}
	if previousCredentialID != "" && previousCredentialID != credentialID {
		keys = append(keys, "codelocal:device:"+previousCredentialID, "codelocal:device-touch:"+previousCredentialID)
	}
	s.invalidateDeviceCache(keys...)
	return &Device{CredentialID: credentialID, PreviousCredentialID: previousCredentialID, UserID: userID, DeviceID: p.DeviceID, DeviceName: p.DeviceName, SecretHash: secretHash, CreatedAt: now, LastSeenAt: now}, nil
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
	err := s.DB.QueryRow(ctx, `SELECT credential_id,user_id,device_id,device_name,secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE credential_id=$1`, credentialID).Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked)
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
	rows, err := s.DB.Query(ctx, `SELECT credential_id,user_id,device_id,device_name,secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE user_id=$1 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var revoked *int64
		if err := rows.Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked); err != nil {
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
	rows, err := s.DB.Query(ctx, `SELECT user_id,device_id,workspace_id,workspace_name,project_root,protocol_version,capabilities,created_at,last_seen_at FROM codelocal_workspaces WHERE user_id=$1 ORDER BY last_seen_at DESC`, userID)
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
		if err := rows.Scan(&w.UserID, &w.DeviceID, &w.WorkspaceID, &w.WorkspaceName, &projectRoot, &protocolVersion, &caps, &w.CreatedAt, &w.LastSeenAt); err != nil {
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
			if days := envInt("CODELOCAL_AUDIT_RETENTION_DAYS", 90); days > 0 {
				_, _ = s.DB.Exec(ctx, `DELETE FROM codelocal_audit_logs WHERE created_at < $1`, time.Now().Add(-time.Duration(days)*24*time.Hour).UnixMilli())
			}
			cancel()
		}
	}
}
