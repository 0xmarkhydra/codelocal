package aipool

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StoredSource struct {
	ID       string
	Name     string
	Kind     string
	BaseURL  string
	APIKey   string
	Priority int
	Enabled  bool
}

type SourceStore struct {
	db   *pgxpool.Pool
	aead cipher.AEAD
}

func NewSourceStore(ctx context.Context, databaseURL, encryptionKey string) (*SourceStore, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required for Pool source storage")
	}
	key, err := decodeEncryptionKey(encryptionKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &SourceStore{db: db, aead: aead}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func decodeEncryptionKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("POOL_ENCRYPTION_KEY is required")
	}
	candidates := [][]byte{}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	if decoded, err := hex.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	candidates = append(candidates, []byte(value))
	for _, candidate := range candidates {
		if len(candidate) == 32 {
			return candidate, nil
		}
	}
	return nil, errors.New("POOL_ENCRYPTION_KEY must decode to exactly 32 bytes")
}

func (s *SourceStore) migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
CREATE SCHEMA IF NOT EXISTS pool;
CREATE TABLE IF NOT EXISTS pool.sources (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key_cipher BYTEA NOT NULL,
  api_key_nonce BYTEA NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS pool_sources_enabled_priority_idx
  ON pool.sources(enabled, priority, id);
`)
	return err
}

func (s *SourceStore) Close() {
	if s != nil && s.db != nil {
		s.db.Close()
	}
}

func randomSourceID() (string, error) {
	buf := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return "src_" + hex.EncodeToString(buf), nil
}

func (s *SourceStore) encrypt(plaintext string) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, s.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = s.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

func (s *SourceStore) decrypt(ciphertext, nonce []byte) (string, error) {
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("failed to decrypt Pool source credential")
	}
	return string(plaintext), nil
}

func (s *SourceStore) Create(ctx context.Context, source StoredSource) (StoredSource, error) {
	if strings.TrimSpace(source.ID) == "" {
		id, err := randomSourceID()
		if err != nil {
			return StoredSource{}, err
		}
		source.ID = id
	}
	if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Kind) == "" || strings.TrimSpace(source.BaseURL) == "" || strings.TrimSpace(source.APIKey) == "" {
		return StoredSource{}, errors.New("source name, kind, base URL, and API key are required")
	}
	if source.Priority < 0 {
		source.Priority = 0
	}
	ciphertext, nonce, err := s.encrypt(source.APIKey)
	if err != nil {
		return StoredSource{}, err
	}
	_, err = s.db.Exec(ctx, `
INSERT INTO pool.sources(id, name, kind, base_url, api_key_cipher, api_key_nonce, priority, enabled)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE SET
  name=EXCLUDED.name,
  kind=EXCLUDED.kind,
  base_url=EXCLUDED.base_url,
  api_key_cipher=EXCLUDED.api_key_cipher,
  api_key_nonce=EXCLUDED.api_key_nonce,
  priority=EXCLUDED.priority,
  enabled=EXCLUDED.enabled,
  updated_at=NOW()
`, source.ID, source.Name, source.Kind, source.BaseURL, ciphertext, nonce, source.Priority, source.Enabled)
	if err != nil {
		return StoredSource{}, err
	}
	return source, nil
}

func (s *SourceStore) List(ctx context.Context, includeDisabled bool) ([]StoredSource, error) {
	query := `SELECT id,name,kind,base_url,api_key_cipher,api_key_nonce,priority,enabled FROM pool.sources`
	if !includeDisabled {
		query += ` WHERE enabled=TRUE`
	}
	query += ` ORDER BY priority ASC, id ASC`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredSource{}
	for rows.Next() {
		var source StoredSource
		var ciphertext, nonce []byte
		if err := rows.Scan(&source.ID, &source.Name, &source.Kind, &source.BaseURL, &ciphertext, &nonce, &source.Priority, &source.Enabled); err != nil {
			return nil, err
		}
		key, err := s.decrypt(ciphertext, nonce)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", source.ID, err)
		}
		source.APIKey = key
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s *SourceStore) SetEnabled(ctx context.Context, id string, enabled bool) error {
	command, err := s.db.Exec(ctx, `UPDATE pool.sources SET enabled=$2, updated_at=NOW() WHERE id=$1`, strings.TrimSpace(id), enabled)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return errors.New("Pool source not found")
	}
	return nil
}

func (s *SourceStore) Delete(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `DELETE FROM pool.sources WHERE id=$1`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return errors.New("Pool source not found")
	}
	return nil
}

func sourceSummaryFromStored(source StoredSource) SourceSummary {
	return SourceSummary{
		ID:        source.ID,
		Name:      source.Name,
		Kind:      source.Kind,
		BaseURL:   source.BaseURL,
		Priority:  source.Priority,
		Enabled:   source.Enabled,
		ManagedBy: "pool",
	}
}

func sourceUpdatedAt() int64 { return time.Now().UnixMilli() }
